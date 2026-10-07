package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
)

var (
	moduleIDPattern    = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[.-][a-z0-9]+)*$`)
	imageDigestPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,447}@sha256:[a-f0-9]{64}$`)
)

type moduleContainerStatus struct {
	State       string
	ContainerID string
}

func performDockerModuleOperation(ctx context.Context, request updaterhelper.Request, serviceUID, serviceGID int) (moduleContainerStatus, string, error) {
	moduleID := strings.TrimSpace(request.ModuleID)
	if !moduleIDPattern.MatchString(moduleID) || len(moduleID) > 96 {
		return moduleContainerStatus{}, "", errors.New("invalid module id")
	}
	if _, err := ensureDockerRuntime(ctx, serviceGID); err != nil {
		return moduleContainerStatus{}, "", err
	}

	switch request.Operation {
	case "docker.module.install":
		image := strings.TrimSpace(request.Image)
		if !imageDigestPattern.MatchString(image) {
			return moduleContainerStatus{}, "", errors.New("module image must be an immutable @sha256 reference")
		}
		return installDockerModule(
			ctx,
			moduleID,
			image,
			serviceUID,
			serviceGID,
			request.ModuleHealthPort,
			request.ModuleHealthPath,
			request.RegistryUsername,
			request.RegistryToken,
		)
	case "docker.module.start":
		return controlDockerModule(ctx, moduleID, "start")
	case "docker.module.stop":
		return controlDockerModule(ctx, moduleID, "stop")
	case "docker.module.restart":
		return controlDockerModule(ctx, moduleID, "restart")
	case "docker.module.remove":
		return removeDockerModule(ctx, moduleID, false)
	case "docker.module.remove-data":
		return removeDockerModule(ctx, moduleID, true)
	default:
		return moduleContainerStatus{}, "", errors.New("unsupported Docker module operation")
	}
}

func installDockerModule(
	ctx context.Context,
	moduleID, image string,
	serviceUID, serviceGID, healthPort int,
	healthPath, registryUser, registryToken string,
) (moduleContainerStatus, string, error) {
	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		return moduleContainerStatus{}, "", errors.New("Docker CLI is unavailable")
	}

	if err := pullDockerImage(ctx, dockerPath, image, registryUser, registryToken); err != nil {
		return moduleContainerStatus{}, "", err
	}

	name := moduleContainerName(moduleID)
	exists, managed, err := inspectManagedContainer(ctx, dockerPath, name, moduleID)
	if err != nil {
		return moduleContainerStatus{}, "", err
	}
	previousImage := ""
	if exists {
		if !managed {
			return moduleContainerStatus{}, "", errors.New("container name is already used by an unmanaged container")
		}
		previousImage, err = inspectModuleContainerImage(ctx, dockerPath, name)
		if err != nil {
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

	if exists {
		if output, err := exec.CommandContext(ctx, dockerPath, "rm", "-f", name).CombinedOutput(); err != nil {
			return moduleContainerStatus{}, "", fmt.Errorf("replace module container: %s", commandError(output, err))
		}
	}

	containerID, err := createDockerModuleContainer(ctx, dockerPath, name, moduleID, image, dataDir, serviceUID, serviceGID)
	if err != nil {
		if previousImage != "" {
			if rollbackErr := rollbackDockerModule(ctx, dockerPath, name, moduleID, previousImage, dataDir, serviceUID, serviceGID); rollbackErr != nil {
				return moduleContainerStatus{}, "", fmt.Errorf("%v; rollback failed: %w", err, rollbackErr)
			}
			return moduleContainerStatus{}, "", fmt.Errorf("%v; previous module image restored", err)
		}
		return moduleContainerStatus{}, "", err
	}

	if output, err := exec.CommandContext(ctx, dockerPath, "start", name).CombinedOutput(); err != nil {
		_ = exec.CommandContext(context.Background(), dockerPath, "rm", "-f", name).Run()
		startErr := fmt.Errorf("start module container: %s", commandError(output, err))
		if previousImage != "" {
			if rollbackErr := rollbackDockerModule(ctx, dockerPath, name, moduleID, previousImage, dataDir, serviceUID, serviceGID); rollbackErr != nil {
				return moduleContainerStatus{}, "", fmt.Errorf("%v; rollback failed: %w", startErr, rollbackErr)
			}
			return moduleContainerStatus{}, "", fmt.Errorf("%v; previous module image restored", startErr)
		}
		return moduleContainerStatus{}, "", startErr
	}

	if err := waitForModuleHealth(ctx, dockerPath, name, healthPort, healthPath); err != nil {
		_ = exec.CommandContext(context.Background(), dockerPath, "rm", "-f", name).Run()
		if previousImage != "" {
			if rollbackErr := rollbackDockerModule(ctx, dockerPath, name, moduleID, previousImage, dataDir, serviceUID, serviceGID); rollbackErr != nil {
				return moduleContainerStatus{}, "", fmt.Errorf("%v; rollback failed: %w", err, rollbackErr)
			}
			return moduleContainerStatus{}, "", fmt.Errorf("%v; previous module image restored", err)
		}
		return moduleContainerStatus{}, "", err
	}

	message := "module installed and started"
	if previousImage != "" && previousImage != image {
		message = "module updated and started"
	}
	return moduleContainerStatus{State: "running", ContainerID: containerID}, message, nil
}

func pullDockerImage(ctx context.Context, dockerPath, image, registryUser, registryToken string) error {
	env := os.Environ()
	if strings.TrimSpace(registryToken) != "" && strings.HasPrefix(strings.ToLower(image), "ghcr.io/") {
		configDir, err := os.MkdirTemp("", "home-ai-docker-auth-*")
		if err != nil {
			return fmt.Errorf("create temporary Docker auth directory: %w", err)
		}
		defer os.RemoveAll(configDir)
		if err := os.Chmod(configDir, 0o700); err != nil {
			return fmt.Errorf("protect temporary Docker auth directory: %w", err)
		}
		user := strings.TrimSpace(registryUser)
		if user == "" {
			user = "DeadSoulf"
		}
		authEnv := append(env, "DOCKER_CONFIG="+configDir)
		login := exec.CommandContext(ctx, dockerPath, "login", "ghcr.io", "--username", user, "--password-stdin")
		login.Env = authEnv
		login.Stdin = strings.NewReader(strings.TrimSpace(registryToken) + "\n")
		if output, err := login.CombinedOutput(); err != nil {
			return fmt.Errorf("authenticate to GHCR: %s", commandError(output, err))
		}
		env = authEnv
	}

	cmd := exec.CommandContext(ctx, dockerPath, "pull", image)
	cmd.Env = env
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("pull module image: %s", commandError(output, err))
	}
	return nil
}

func createDockerModuleContainer(
	ctx context.Context,
	dockerPath, name, moduleID, image, dataDir string,
	serviceUID, serviceGID int,
) (string, error) {
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
		image,
	}
	output, err := exec.CommandContext(ctx, dockerPath, args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("create module container: %s", commandError(output, err))
	}
	return strings.TrimSpace(string(output)), nil
}

func rollbackDockerModule(
	ctx context.Context,
	dockerPath, name, moduleID, image, dataDir string,
	serviceUID, serviceGID int,
) error {
	_ = exec.CommandContext(context.Background(), dockerPath, "rm", "-f", name).Run()
	if _, err := createDockerModuleContainer(ctx, dockerPath, name, moduleID, image, dataDir, serviceUID, serviceGID); err != nil {
		return err
	}
	if output, err := exec.CommandContext(ctx, dockerPath, "start", name).CombinedOutput(); err != nil {
		return fmt.Errorf("restart previous module container: %s", commandError(output, err))
	}
	return nil
}

func waitForModuleHealth(ctx context.Context, dockerPath, name string, port int, path string) error {
	if port == 0 && strings.TrimSpace(path) == "" {
		status, err := inspectModuleContainer(ctx, dockerPath, name)
		if err != nil {
			return err
		}
		if status.State != "running" {
			return fmt.Errorf("module container is not running: %s", status.State)
		}
		return nil
	}
	if port < 1 || port > 65535 || !strings.HasPrefix(path, "/") {
		return errors.New("invalid module health endpoint")
	}

	ipOutput, err := exec.CommandContext(
		ctx,
		dockerPath,
		"inspect",
		"--format",
		`{{with index .NetworkSettings.Networks "home-ai-modules"}}{{.IPAddress}}{{end}}`,
		name,
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("inspect module health address: %s", commandError(ipOutput, err))
	}
	ip := strings.TrimSpace(string(ipOutput))
	if net.ParseIP(ip) == nil {
		return errors.New("module health address is unavailable")
	}
	url := "http://" + net.JoinHostPort(ip, strconv.Itoa(port)) + path
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(30 * time.Second)
	var lastError error
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 400 {
				return nil
			}
			lastError = fmt.Errorf("health endpoint returned HTTP %d", resp.StatusCode)
		} else {
			lastError = err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	if lastError == nil {
		lastError = errors.New("health endpoint did not become ready")
	}
	return fmt.Errorf("module health check failed: %w", lastError)
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

func removeDockerModule(ctx context.Context, moduleID string, removeData bool) (moduleContainerStatus, string, error) {
	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		return moduleContainerStatus{}, "", errors.New("Docker CLI is unavailable")
	}
	name := moduleContainerName(moduleID)
	exists, managed, err := inspectManagedContainer(ctx, dockerPath, name, moduleID)
	if err != nil {
		return moduleContainerStatus{}, "", err
	}
	if exists {
		if !managed {
			return moduleContainerStatus{}, "", errors.New("refusing to remove unmanaged container")
		}
		output, err := exec.CommandContext(ctx, dockerPath, "rm", "-f", name).CombinedOutput()
		if err != nil {
			return moduleContainerStatus{}, "", fmt.Errorf("remove module container: %s", commandError(output, err))
		}
	}
	if removeData {
		dataDir := filepath.Join(moduleDataRoot, moduleID)
		if err := os.RemoveAll(dataDir); err != nil {
			return moduleContainerStatus{}, "", fmt.Errorf("remove module persistent data: %w", err)
		}
		return moduleContainerStatus{State: "removed"}, "module container and persistent data removed", nil
	}
	if !exists {
		return moduleContainerStatus{State: "removed"}, "module container already absent; data preserved", nil
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

func inspectModuleContainerImage(ctx context.Context, dockerPath, name string) (string, error) {
	output, err := exec.CommandContext(ctx, dockerPath, "inspect", "--format", `{{ .Config.Image }}`, name).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("inspect module image: %s", commandError(output, err))
	}
	image := strings.TrimSpace(string(output))
	if !imageDigestPattern.MatchString(image) {
		return "", errors.New("installed module image is not pinned to an immutable digest")
	}
	return image, nil
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
