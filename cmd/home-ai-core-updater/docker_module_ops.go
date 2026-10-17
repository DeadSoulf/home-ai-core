package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
)

var (
	moduleOperationMu   sync.Mutex
	moduleIDPattern     = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[.-][a-z0-9]+)*$`)
	imageDigestPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,447}@sha256:[a-f0-9]{64}$`)
	localImageIDPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
)

// The official NVR digest was once published successfully but is not now
// anonymously readable. Only this exact historical digest can fall back
// to a local build from the *same immutable source revision*. Arbitrary
// manifest image references can never choose a source URL.
const (
	nvrLegacyImage        = "ghcr.io/deadsoulf/home-ai-core-nvr@sha256:9f89c074832380b9e7cf9cdd41454ceffa5c2bdb364cdb6be45f02de6f9910d1"
	nvrPinnedRevision     = "1793b39553c4aac37fd7a468db93e170c9e616d2"
	nvrPinnedRepository   = "https://github.com/DeadSoulf/home-ai-core_dvr"
	nvrPinnedBuildContext = nvrPinnedRepository + ".git#" + nvrPinnedRevision
	nvrPinnedLocalTag     = "home-ai-core-local-nvr:1.1.1-1793b39553c4"
)

type moduleContainerStatus struct {
	State       string
	ContainerID string
}

func performDockerModuleOperation(ctx context.Context, request updaterhelper.Request, serviceUID, serviceGID int) (moduleContainerStatus, string, error) {
	if !moduleOperationMu.TryLock() {
		return moduleContainerStatus{}, "", errors.New("another Docker module operation is already in progress")
	}
	defer moduleOperationMu.Unlock()
	moduleID := strings.TrimSpace(request.ModuleID)
	if !moduleIDPattern.MatchString(moduleID) || len(moduleID) > 96 {
		return moduleContainerStatus{}, "", errors.New("invalid module id")
	}
	// Reject malformed or unsupported operations before starting or installing Docker.
	// Validation must not trigger privileged system changes.
	switch request.Operation {
	case "docker.module.install":
		if !imageDigestPattern.MatchString(strings.TrimSpace(request.Image)) {
			return moduleContainerStatus{}, "", errors.New("module image must be an immutable @sha256 reference")
		}
	case "docker.module.start", "docker.module.stop", "docker.module.restart", "docker.module.remove":
	default:
		return moduleContainerStatus{}, "", errors.New("unsupported Docker module operation")
	}
	if request.Operation == "docker.module.install" {
		if _, err := ensureDockerRuntime(ctx, serviceGID); err != nil {
			return moduleContainerStatus{}, "", err
		}
	} else {
		// Control/remove must never install Docker or change host services.
		status := inspectDockerRuntime(ctx)
		if !moduleRuntimeAllows(request.Operation, status) {
			return moduleContainerStatus{}, "", fmt.Errorf("Docker runtime is not ready: %s", status.Error)
		}
	}

	switch request.Operation {
	case "docker.module.install":
		return installDockerModule(ctx, moduleID, strings.TrimSpace(request.Image), serviceUID, serviceGID)
	case "docker.module.start":
		return controlDockerModule(ctx, moduleID, "start")
	case "docker.module.stop":
		return controlDockerModule(ctx, moduleID, "stop")
	case "docker.module.restart":
		return controlDockerModule(ctx, moduleID, "restart")
	case "docker.module.remove":
		return removeDockerModule(ctx, moduleID)
	default:
		return moduleContainerStatus{}, "", errors.New("unsupported Docker module operation")
	}
}

func installDockerModule(ctx context.Context, moduleID, image string, serviceUID, serviceGID int) (moduleContainerStatus, string, error) {
	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		return moduleContainerStatus{}, "", errors.New("Docker CLI is unavailable")
	}

	name := moduleContainerName(moduleID)
	if exists, managed, err := inspectManagedContainer(ctx, dockerPath, name, moduleID); err != nil {
		return moduleContainerStatus{}, "", err
	} else if exists {
		if !managed {
			return moduleContainerStatus{}, "", errors.New("container name is already used by an unmanaged container")
		}
		// Never destroy a running module before a candidate has passed start,
		// health and recovery checks. Transactional upgrades are a separate path.
		return resumeManagedModule(ctx, dockerPath, name, image)
	}

	installImage, fromPinnedSource, err := resolveInstallModuleImage(ctx, dockerPath, moduleID, image)
	if err != nil {
		return moduleContainerStatus{}, "", err
	}

	if moduleDataRoot == "/var/lib/home-ai-modules" {
		legacy := filepath.Join("/var/lib/home-ai-core/modules", moduleID)
		if _, err := os.Lstat(legacy); err == nil {
			return moduleContainerStatus{}, "", errors.New("legacy module data exists; migrate it to the protected module data root before a new install (existing containers can still be resumed)")
		} else if !errors.Is(err, os.ErrNotExist) {
			return moduleContainerStatus{}, "", err
		}
	}
	dataDir := filepath.Join(moduleDataRoot, moduleID)
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		return moduleContainerStatus{}, "", fmt.Errorf("create module data directory: %w", err)
	}
	if err := os.Chown(dataDir, serviceUID, serviceGID); err != nil {
		return moduleContainerStatus{}, "", fmt.Errorf("set module data ownership: %w", err)
	}
	if err := os.Chmod(dataDir, 0o700); err != nil {
		return moduleContainerStatus{}, "", fmt.Errorf("set module data permissions: %w", err)
	}

	args := []string{
		"create",
		"--name", name,
		"--label", "home-ai.managed=true",
		"--label", "home-ai.module.id=" + moduleID,
		"--network", dockerNetworkName,
		"--user", fmt.Sprintf("%d:%d", serviceUID, serviceGID),
		"--restart", "unless-stopped",
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges",
		"--pids-limit", "512",
		"--read-only",
		"--tmpfs", "/tmp:rw,noexec,nosuid,size=64m",
		"--mount", "type=bind,src=" + dataDir + ",dst=/data",
	}
	if fromPinnedSource {
		args = append(args,
			"--label", "home-ai.module.requested-image="+image,
			"--label", "home-ai.module.source-revision="+nvrPinnedRevision,
		)
	}
	args = append(args, installImage)
	output, err := exec.CommandContext(ctx, dockerPath, args...).CombinedOutput()
	if err != nil {
		return moduleContainerStatus{}, "", fmt.Errorf("create module container: %s", commandError(output, err))
	}
	containerID := strings.TrimSpace(string(output))

	if output, err := exec.CommandContext(ctx, dockerPath, "start", name).CombinedOutput(); err != nil {
		return moduleContainerStatus{}, "", cleanupFailedModuleStart(dockerPath, name, fmt.Errorf("start module container: %s", commandError(output, err)))
	}
	// Docker 'start' can return success when the container process exits
	// immediately. Never publish such an instance as an enabled module.
	select {
	case <-ctx.Done():
		return moduleContainerStatus{}, "", cleanupFailedModuleStart(dockerPath, name, ctx.Err())
	case <-time.After(300 * time.Millisecond):
	}
	status, err := inspectModuleContainer(ctx, dockerPath, name)
	if err != nil {
		return moduleContainerStatus{}, "", cleanupFailedModuleStart(dockerPath, name, fmt.Errorf("inspect started module: %w", err))
	}
	if status.State != "running" {
		return moduleContainerStatus{}, "", cleanupFailedModuleStart(dockerPath, name, fmt.Errorf("module stopped during startup (state: %s)", status.State))
	}
	message := "module installed and started"
	if fromPinnedSource {
		message = "module built from pinned official NVR source and started; GHCR image was not anonymously available"
	}
	return moduleContainerStatus{State: status.State, ContainerID: containerID}, message, nil
}

func resolveInstallModuleImage(ctx context.Context, dockerPath, moduleID, manifestImage string) (string, bool, error) {
	pullErr := pullDockerModuleImage(ctx, dockerPath, manifestImage)
	if pullErr == nil {
		return manifestImage, false, nil
	}
	if moduleID != "nvr.module" || manifestImage != nvrLegacyImage || !nvrRegistrySourceFallbackAllowed(pullErr) {
		return "", false, pullErr
	}
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		return "", false, fmt.Errorf("pinned NVR source build requires Linux amd64: %w", pullErr)
	}
	localImage, err := buildPinnedNVRImage(ctx, dockerPath)
	if err != nil {
		return "", false, fmt.Errorf("NVR image access was denied in GHCR (%v); build from pinned GitHub source failed: %w", pullErr, err)
	}
	return localImage, true, nil
}

func nvrRegistrySourceFallbackAllowed(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, fragment := range []string{
		"unauthorized", "authentication required", "insufficient_scope",
		"manifest unknown", "manifest not found", "denied: requested access",
	} {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}

func buildPinnedNVRImage(ctx context.Context, dockerPath string) (string, error) {
	// The updater runs as root inside a hardened systemd unit (ProtectHome).
	// Docker/Buildx must never attempt to write /root/.docker. Give both CLI
	// calls an isolated, writable, temporary HOME and DOCKER_CONFIG instead
	// of relaxing systemd protections or persisting registry credentials.
	cliHome, err := os.MkdirTemp("", "home-ai-nvr-docker-*")
	if err != nil {
		return "", fmt.Errorf("create temporary NVR Docker home: %w", err)
	}
	defer os.RemoveAll(cliHome)
	dockerConfig := filepath.Join(cliHome, ".docker")
	if err := os.Mkdir(dockerConfig, 0o700); err != nil {
		return "", fmt.Errorf("create temporary NVR Docker config: %w", err)
	}
	dockerEnv := append(os.Environ(), "HOME="+cliHome, "DOCKER_CONFIG="+dockerConfig)

	// Docker's remote Git context is pinned to a full 40-digit commit SHA.
	// No shell is invoked, mutable refs are not used, and the resulting
	// container uses the local content-addressed ID rather than a tag.
	build := exec.CommandContext(
		ctx, dockerPath, "build",
		"--tag", nvrPinnedLocalTag,
		"--label", "org.opencontainers.image.source="+nvrPinnedRepository,
		"--label", "org.opencontainers.image.revision="+nvrPinnedRevision,
		nvrPinnedBuildContext,
	)
	build.Env = dockerEnv
	output, err := build.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("build trusted NVR source: %s", commandError(output, err))
	}
	inspect := exec.CommandContext(
		ctx, dockerPath, "image", "inspect", "--format", "{{.Id}}", nvrPinnedLocalTag,
	)
	inspect.Env = dockerEnv
	output, err = inspect.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("inspect locally built NVR image: %s", commandError(output, err))
	}
	imageID := strings.TrimSpace(string(output))
	if !localImageIDPattern.MatchString(imageID) {
		return "", errors.New("locally built NVR image has an invalid immutable SHA-256 image ID")
	}
	return imageID, nil
}

func pullDockerModuleImage(ctx context.Context, dockerPath, image string) error {
	token := strings.TrimSpace(os.Getenv("HOME_AI_MODULE_CATALOG_TOKEN"))
	lowerImage := strings.ToLower(image)
	if token == "" || !strings.HasPrefix(lowerImage, "ghcr.io/") {
		if output, err := exec.CommandContext(ctx, dockerPath, "pull", image).CombinedOutput(); err != nil {
			return fmt.Errorf("pull module image: %s", commandError(output, err))
		}
		return nil
	}

	user := strings.TrimSpace(os.Getenv("HOME_AI_MODULE_REGISTRY_USER"))
	if user == "" {
		user = "DeadSoulf"
	}
	configDir, err := os.MkdirTemp("", "home-ai-docker-auth-*")
	if err != nil {
		return fmt.Errorf("create temporary Docker auth directory: %w", err)
	}
	defer os.RemoveAll(configDir)

	env := append(os.Environ(), "DOCKER_CONFIG="+configDir)
	login := exec.CommandContext(ctx, dockerPath, "login", "ghcr.io", "--username", user, "--password-stdin")
	login.Env = env
	login.Stdin = strings.NewReader(token)
	if output, err := login.CombinedOutput(); err != nil {
		return fmt.Errorf("authenticate to private module registry: %s", commandError(output, err))
	}
	pull := exec.CommandContext(ctx, dockerPath, "pull", image)
	pull.Env = env
	if output, err := pull.CombinedOutput(); err != nil {
		return fmt.Errorf("pull module image: %s", commandError(output, err))
	}
	return nil
}

func controlDockerModule(ctx context.Context, moduleID, action string) (moduleContainerStatus, string, error) {
	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		return moduleContainerStatus{}, "", errors.New("Docker CLI is unavailable")
	}
	name := moduleContainerName(moduleID)
	exists, managed, err := inspectManagedContainer(ctx, dockerPath, name, moduleID)
	if err != nil {
		return moduleContainerStatus{}, "", err
	}
	if !exists {
		return moduleContainerStatus{}, "", errors.New("module container is not installed")
	}
	if !managed {
		return moduleContainerStatus{}, "", errors.New("refusing to control unmanaged container")
	}
	output, err := exec.CommandContext(ctx, dockerPath, action, name).CombinedOutput()
	if err != nil {
		return moduleContainerStatus{}, "", fmt.Errorf("%s module container: %s", action, commandError(output, err))
	}
	status, err := inspectModuleContainer(ctx, dockerPath, name)
	if err != nil {
		return moduleContainerStatus{}, "", err
	}
	return status, "module " + action + " completed", nil
}

func removeDockerModule(ctx context.Context, moduleID string) (moduleContainerStatus, string, error) {
	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		return moduleContainerStatus{}, "", errors.New("Docker CLI is unavailable")
	}
	name := moduleContainerName(moduleID)
	exists, managed, err := inspectManagedContainer(ctx, dockerPath, name, moduleID)
	if err != nil {
		return moduleContainerStatus{}, "", err
	}
	if !exists {
		return moduleContainerStatus{State: "removed"}, "module container already absent; data preserved", nil
	}
	if !managed {
		return moduleContainerStatus{}, "", errors.New("refusing to remove unmanaged container")
	}
	output, err := exec.CommandContext(ctx, dockerPath, "rm", "-f", name).CombinedOutput()
	if err != nil {
		return moduleContainerStatus{}, "", fmt.Errorf("remove module container: %s", commandError(output, err))
	}
	return moduleContainerStatus{State: "removed"}, "module container removed; data preserved", nil
}

func inspectManagedContainer(ctx context.Context, dockerPath, name, moduleID string) (bool, bool, error) {
	output, err := exec.CommandContext(
		ctx,
		dockerPath,
		"inspect",
		"--format",
		`{{ index .Config.Labels "home-ai.managed" }}|{{ index .Config.Labels "home-ai.module.id" }}`,
		name,
	).CombinedOutput()
	if err != nil {
		message := strings.ToLower(string(output))
		if strings.Contains(message, "no such") {
			return false, false, nil
		}
		return false, false, fmt.Errorf("inspect module container: %s", commandError(output, err))
	}
	parts := strings.SplitN(strings.TrimSpace(string(output)), "|", 2)
	return true, len(parts) == 2 && parts[0] == "true" && parts[1] == moduleID, nil
}

func inspectModuleContainer(ctx context.Context, dockerPath, name string) (moduleContainerStatus, error) {
	output, err := exec.CommandContext(
		ctx,
		dockerPath,
		"inspect",
		"--format",
		`{{ .Id }}|{{ .State.Status }}`,
		name,
	).CombinedOutput()
	if err != nil {
		return moduleContainerStatus{}, fmt.Errorf("inspect module state: %s", commandError(output, err))
	}
	parts := strings.SplitN(strings.TrimSpace(string(output)), "|", 2)
	if len(parts) != 2 {
		return moduleContainerStatus{}, errors.New("invalid Docker module state")
	}
	return moduleContainerStatus{ContainerID: parts[0], State: parts[1]}, nil
}

func moduleContainerName(moduleID string) string {
	return "home-ai-module-" + moduleID
}

// cleanupFailedModuleStart bounds cleanup even if the parent job was canceled.
// Preserve cleanup failures in the returned error for operator recovery.
func cleanupFailedModuleStart(dockerPath, name string, cause error) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, dockerPath, "rm", "-f", name).CombinedOutput()
	if err != nil {
		return errors.Join(cause, fmt.Errorf("remove failed module container: %s", commandError(output, err)))
	}
	return cause
}

// Stop/removal and compensation must remain available when only the bridge
// network is broken. They still verify managed-container ownership below.
func moduleRuntimeAllows(operation string, status dockerRuntimeStatus) bool {
	if !status.Available || !status.Active {
		return false
	}
	if operation == "docker.module.stop" || operation == "docker.module.remove" {
		return true
	}
	return status.NetworkReady
}

func resumeManagedModule(ctx context.Context, dockerPath, name, image string) (moduleContainerStatus, string, error) {
	output, err := exec.CommandContext(ctx, dockerPath, "inspect", "--format", "{{json .}}", name).CombinedOutput()
	if err != nil {
		return moduleContainerStatus{}, "", fmt.Errorf("inspect interrupted module: %s", commandError(output, err))
	}
	var info struct {
		ID     string `json:"Id"`
		Image  string `json:"Image"`
		Config struct {
			Image  string            `json:"Image"`
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
		State struct {
			Status string `json:"Status"`
			Health *struct {
				Status string `json:"Status"`
			} `json:"Health"`
		} `json:"State"`
	}
	if err := json.Unmarshal(output, &info); err != nil {
		return moduleContainerStatus{}, "", fmt.Errorf("invalid interrupted module state: %w", err)
	}
	originalImageMatches := info.Config.Image == image
	pinnedSourceMatches := image == nvrLegacyImage &&
		localImageIDPattern.MatchString(info.Config.Image) &&
		info.Image == info.Config.Image &&
		info.Config.Labels["home-ai.module.requested-image"] == nvrLegacyImage &&
		info.Config.Labels["home-ai.module.source-revision"] == nvrPinnedRevision
	if info.ID == "" || (!originalImageMatches && !pinnedSourceMatches) {
		return moduleContainerStatus{}, "", errors.New("existing managed container has a different image; in-place upgrade is disabled")
	}
	if info.State.Health != nil && info.State.Health.Status != "healthy" {
		return moduleContainerStatus{}, "", errors.New("existing container is not healthy; repair or remove it before retrying")
	}
	switch info.State.Status {
	case "running":
	case "created", "exited":
		if output, err := exec.CommandContext(ctx, dockerPath, "start", name).CombinedOutput(); err != nil {
			return moduleContainerStatus{}, "", fmt.Errorf("resume module: %s", commandError(output, err))
		}
	default:
		return moduleContainerStatus{}, "", fmt.Errorf("cannot adopt a module in state %q", info.State.Status)
	}
	select {
	case <-ctx.Done():
		return moduleContainerStatus{}, "", ctx.Err()
	case <-time.After(300 * time.Millisecond):
	}
	status, err := inspectModuleContainer(ctx, dockerPath, name)
	if err != nil {
		return moduleContainerStatus{}, "", err
	}
	if status.State != "running" {
		return moduleContainerStatus{}, "", fmt.Errorf("resumed module is not running (%s)", status.State)
	}
	return status, "same-image module installation recovered; persistent data preserved", nil
}
