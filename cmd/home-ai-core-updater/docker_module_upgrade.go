package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"
)

func validateModuleHealthCheck(port int, path string) error {
	if port < 1 || port > 65535 {
		return errors.New("module upgrade requires a valid health port")
	}
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") ||
		len(path) > 200 || strings.Contains(path, "..") ||
		strings.ContainsAny(path, "?#\\\r\n\t") {
		return errors.New("module upgrade requires a safe HTTP health path")
	}
	return nil
}

func upgradeDockerModule(ctx context.Context, moduleID, image, previousImage string, healthPort int, healthPath string, serviceUID, serviceGID int) (moduleContainerStatus, string, error) {
	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		return moduleContainerStatus{}, "", errors.New("Docker CLI is unavailable")
	}
	name := moduleContainerName(moduleID)
	backup := moduleRollbackContainerName(moduleID)
	exists, managed, err := inspectManagedContainer(ctx, dockerPath, name, moduleID)
	if err != nil {
		return moduleContainerStatus{}, "", err
	}
	if !exists || !managed {
		return moduleContainerStatus{}, "", errors.New("upgrade requires an existing managed module container")
	}
	backupExists, _, err := inspectManagedContainer(ctx, dockerPath, backup, moduleID)
	if err != nil {
		return moduleContainerStatus{}, "", err
	}
	if backupExists {
		return moduleContainerStatus{}, "", errors.New("a previous upgrade backup is pending; resolve it before another upgrade")
	}
	output, err := exec.CommandContext(ctx, dockerPath, "inspect", "--format", "{{.Config.Image}}", name).CombinedOutput()
	if err != nil {
		return moduleContainerStatus{}, "", fmt.Errorf("inspect previous image: %s", commandError(output, err))
	}
	if strings.TrimSpace(string(output)) != previousImage {
		return moduleContainerStatus{}, "", errors.New("running module image does not match the installed immutable digest; refusing upgrade")
	}
	if err := pullDockerModuleImage(ctx, dockerPath, image); err != nil {
		return moduleContainerStatus{}, "", err
	}
	if output, err := exec.CommandContext(ctx, dockerPath, "stop", "-t", "20", name).CombinedOutput(); err != nil {
		return moduleContainerStatus{}, "", fmt.Errorf("stop previous container: %s", commandError(output, err))
	}
	if output, err := exec.CommandContext(ctx, dockerPath, "rename", name, backup).CombinedOutput(); err != nil {
		// Rename failed before the replacement exists; restore service immediately.
		restoreCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, _ = exec.CommandContext(restoreCtx, dockerPath, "start", name).CombinedOutput()
		return moduleContainerStatus{}, "", fmt.Errorf("preserve previous container: %s", commandError(output, err))
	}

	status, _, installErr := installDockerModule(ctx, moduleID, image, serviceUID, serviceGID)
	if installErr == nil {
		status.IP, installErr = inspectModuleNetworkIP(ctx, dockerPath, name)
	}
	if installErr != nil {
		restoreCtx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		if _, _, rollbackErr := rollbackDockerModule(restoreCtx, moduleID); rollbackErr != nil {
			return moduleContainerStatus{}, "", errors.Join(installErr, fmt.Errorf("automatic rollback failed; previous container retained as %s: %w", backup, rollbackErr))
		}
		return moduleContainerStatus{}, "", fmt.Errorf("replacement failed health verification; previous container restored: %w", installErr)
	}
	// Do not delete the previous container here. The registry must commit the
	// new manifest first, then explicitly finalize this operation.
	return status, "module replacement running; awaiting health verification and registry commit", nil
}

func moduleRollbackContainerName(moduleID string) string {
	return moduleContainerName(moduleID) + "-previous"
}

func rollbackDockerModule(ctx context.Context, moduleID string) (moduleContainerStatus, string, error) {
	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		return moduleContainerStatus{}, "", errors.New("Docker CLI is unavailable")
	}
	name := moduleContainerName(moduleID)
	backup := moduleRollbackContainerName(moduleID)
	backupExists, backupManaged, err := inspectManagedContainer(ctx, dockerPath, backup, moduleID)
	if err != nil {
		return moduleContainerStatus{}, "", err
	}
	if !backupExists || !backupManaged {
		return moduleContainerStatus{}, "", errors.New("no managed previous container is available for rollback")
	}
	exists, managed, err := inspectManagedContainer(ctx, dockerPath, name, moduleID)
	if err != nil {
		return moduleContainerStatus{}, "", err
	}
	if exists {
		if !managed {
			return moduleContainerStatus{}, "", errors.New("refusing to replace an unmanaged container during rollback")
		}
		if output, err := exec.CommandContext(ctx, dockerPath, "rm", "-f", name).CombinedOutput(); err != nil {
			return moduleContainerStatus{}, "", fmt.Errorf("remove failed replacement: %s", commandError(output, err))
		}
	}
	if output, err := exec.CommandContext(ctx, dockerPath, "rename", backup, name).CombinedOutput(); err != nil {
		return moduleContainerStatus{}, "", fmt.Errorf("restore previous container name: %s", commandError(output, err))
	}
	if output, err := exec.CommandContext(ctx, dockerPath, "start", name).CombinedOutput(); err != nil {
		return moduleContainerStatus{}, "", fmt.Errorf("start restored container: %s", commandError(output, err))
	}
	status, err := inspectModuleContainer(ctx, dockerPath, name)
	if err != nil {
		return moduleContainerStatus{}, "", err
	}
	return status, "previous container restored; module data retained", nil
}

func finalizeDockerModule(ctx context.Context, moduleID string) (moduleContainerStatus, string, error) {
	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		return moduleContainerStatus{}, "", errors.New("Docker CLI is unavailable")
	}
	name := moduleContainerName(moduleID)
	backup := moduleRollbackContainerName(moduleID)
	exists, managed, err := inspectManagedContainer(ctx, dockerPath, name, moduleID)
	if err != nil {
		return moduleContainerStatus{}, "", err
	}
	if !exists || !managed {
		return moduleContainerStatus{}, "", errors.New("cannot finalize upgrade without current managed module")
	}
	backupExists, backupManaged, err := inspectManagedContainer(ctx, dockerPath, backup, moduleID)
	if err != nil {
		return moduleContainerStatus{}, "", err
	}
	if !backupExists || !backupManaged {
		return moduleContainerStatus{}, "", errors.New("cannot finalize upgrade without previous managed container")
	}
	if output, err := exec.CommandContext(ctx, dockerPath, "rm", backup).CombinedOutput(); err != nil {
		return moduleContainerStatus{}, "", fmt.Errorf("clean previous container: %s", commandError(output, err))
	}
	status, err := inspectModuleContainer(ctx, dockerPath, name)
	if err != nil {
		return moduleContainerStatus{}, "", err
	}
	return status, "verified module upgrade committed", nil
}

// The helper only resolves a container IP on the managed Docker bridge; it
// cannot make HTTP requests because its systemd sandbox allows AF_UNIX only.
func inspectModuleNetworkIP(ctx context.Context, dockerPath, name string) (string, error) {
	output, err := exec.CommandContext(ctx, dockerPath, "inspect", "--format",
		`{{ with index .NetworkSettings.Networks "home-ai-modules" }}{{ .IPAddress }}{{ end }}`, name).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("inspect managed Docker network: %s", commandError(output, err))
	}
	ip := strings.TrimSpace(string(output))
	parsed := net.ParseIP(ip)
	if parsed == nil || !parsed.IsPrivate() {
		return "", errors.New("managed Docker module does not have a private network IP")
	}
	return ip, nil
}
