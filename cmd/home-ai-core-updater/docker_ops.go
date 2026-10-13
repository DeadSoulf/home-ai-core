package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	dockerServiceName = "docker.service"
	dockerNetworkName = "home-ai-modules"
	moduleDataRoot    = "/var/lib/home-ai-core/modules"
)

type dockerRuntimeStatus struct {
	Available    bool
	Active       bool
	Version      string
	NetworkReady bool
	Error        string
}

func inspectDockerRuntime(ctx context.Context) dockerRuntimeStatus {
	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		return dockerRuntimeStatus{Error: "Docker is not installed"}
	}

	status := dockerRuntimeStatus{Available: true}
	if exec.CommandContext(ctx, "/usr/bin/systemctl", "is-active", "--quiet", dockerServiceName).Run() != nil {
		status.Error = "Docker service is inactive"
		return status
	}
	status.Active = true

	output, err := exec.CommandContext(ctx, dockerPath, "version", "--format", "{{.Server.Version}}").CombinedOutput()
	if err != nil {
		status.Error = "Docker engine is unavailable: " + commandError(output, err)
		return status
	}
	status.Version = strings.TrimSpace(string(output))

	label, err := exec.CommandContext(
		ctx,
		dockerPath,
		"network",
		"inspect",
		"--format",
		"{{ index .Labels \"home-ai.managed\" }}",
		dockerNetworkName,
	).CombinedOutput()
	if err != nil {
		status.Error = "Home-AI module network is not ready"
		return status
	}
	if strings.TrimSpace(string(label)) != "true" {
		status.Error = "Docker network home-ai-modules exists but is not managed by Home-AI"
		return status
	}
	status.NetworkReady = true
	return status
}

func ensureDockerRuntime(ctx context.Context, serviceGID int) (dockerRuntimeStatus, error) {
	if _, err := exec.LookPath("docker"); err != nil {
		if err := installDockerPackage(ctx); err != nil {
			return inspectDockerRuntime(ctx), err
		}
	}

	output, err := exec.CommandContext(ctx, "/usr/bin/systemctl", "enable", "--now", dockerServiceName).CombinedOutput()
	if err != nil {
		return inspectDockerRuntime(ctx), fmt.Errorf("enable Docker service: %s", commandError(output, err))
	}
	if err := waitForDockerService(ctx, 30*time.Second); err != nil {
		return inspectDockerRuntime(ctx), err
	}

	if err := os.MkdirAll(moduleDataRoot, 0o750); err != nil {
		return inspectDockerRuntime(ctx), fmt.Errorf("create module data root: %w", err)
	}
	if err := os.Chown(moduleDataRoot, 0, serviceGID); err != nil {
		return inspectDockerRuntime(ctx), fmt.Errorf("set module data ownership: %w", err)
	}
	if err := os.Chmod(moduleDataRoot, 0o750); err != nil {
		return inspectDockerRuntime(ctx), fmt.Errorf("set module data permissions: %w", err)
	}

	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		return inspectDockerRuntime(ctx), errors.New("Docker installation completed but docker CLI is unavailable")
	}
	label, inspectErr := exec.CommandContext(
		ctx,
		dockerPath,
		"network",
		"inspect",
		"--format",
		"{{ index .Labels \"home-ai.managed\" }}",
		dockerNetworkName,
	).CombinedOutput()
	if inspectErr == nil {
		if strings.TrimSpace(string(label)) != "true" {
			return inspectDockerRuntime(ctx), errors.New("Docker network home-ai-modules already exists and is not managed by Home-AI")
		}
		return inspectDockerRuntime(ctx), nil
	}

	output, err = exec.CommandContext(
		ctx,
		dockerPath,
		"network",
		"create",
		"--driver",
		"bridge",
		"--label",
		"home-ai.managed=true",
		dockerNetworkName,
	).CombinedOutput()
	if err != nil {
		return inspectDockerRuntime(ctx), fmt.Errorf("create Home-AI Docker network: %s", commandError(output, err))
	}

	status := inspectDockerRuntime(ctx)
	if !status.Available || !status.Active || !status.NetworkReady {
		if status.Error == "" {
			status.Error = "Docker runtime did not become ready"
		}
		return status, errors.New(status.Error)
	}
	return status, nil
}

func installDockerPackage(ctx context.Context) error {
	if !packageInstallMu.TryLock() {
		return errors.New("package installation is already in progress")
	}
	defer packageInstallMu.Unlock()

	apt, err := exec.LookPath("apt-get")
	if err != nil {
		return errors.New("apt-get is unavailable")
	}
	systemdRun, err := exec.LookPath("systemd-run")
	if err != nil {
		return errors.New("systemd-run is unavailable")
	}
	for _, args := range [][]string{{"update"}, {"install", "-y", "docker.io"}} {
		cmd := exec.CommandContext(ctx, systemdRun, transientPackageCommand(apt, args...)...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("install Docker: %s", commandError(output, err))
		}
	}
	return nil
}

func waitForDockerService(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if exec.CommandContext(ctx, "/usr/bin/systemctl", "is-active", "--quiet", dockerServiceName).Run() == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return errors.New("Docker service did not become active")
}

func commandError(output []byte, err error) string {
	message := strings.TrimSpace(string(output))
	if message != "" {
		return message
	}
	return err.Error()
}
