package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
)

var (
	moduleIDPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[.-][a-z0-9]+)*$`)
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
		return installDockerModule(ctx, moduleID, image, serviceUID, serviceGID)
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

	if output, err := exec.CommandContext(ctx, dockerPath, "pull", image).CombinedOutput(); err != nil {
		return moduleContainerStatus{}, "", fmt.Errorf("pull module image: %s", commandError(output, err))
	}

	name := moduleContainerName(moduleID)
	if exists, managed, err := inspectManagedContainer(ctx, dockerPath, name, moduleID); err != nil {
		return moduleContainerStatus{}, "", err
	} else if exists {
		if !managed {
			return moduleContainerStatus{}, "", errors.New("container name is already used by an unmanaged container")
		}
		if output, err := exec.CommandContext(ctx, dockerPath, "rm", "-f", name).CombinedOutput(); err != nil {
			return moduleContainerStatus{}, "", fmt.Errorf("replace module container: %s", commandError(output, err))
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
		image,
	}
	output, err := exec.CommandContext(ctx, dockerPath, args...).CombinedOutput()
	if err != nil {
		return moduleContainerStatus{}, "", fmt.Errorf("create module container: %s", commandError(output, err))
	}
	containerID := strings.TrimSpace(string(output))

	if output, err := exec.CommandContext(ctx, dockerPath, "start", name).CombinedOutput(); err != nil {
		_ = exec.CommandContext(context.Background(), dockerPath, "rm", "-f", name).Run()
		return moduleContainerStatus{}, "", fmt.Errorf("start module container: %s", commandError(output, err))
	}

	return moduleContainerStatus{State: "running", ContainerID: containerID}, "module installed and started", nil
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
