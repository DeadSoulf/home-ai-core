package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
)

func performNVROperation(ctx context.Context, request updaterhelper.Request) (string, error) {
	switch request.Operation {
	case "nvr.runtime.install":
		return installNVRMediaRuntime(ctx)
	default:
		return "", errors.New("unsupported NVR operation")
	}
}

func nvrMediaToolsAvailable() bool {
	for _, name := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(name); err != nil {
			return false
		}
	}
	return true
}

func installNVRMediaRuntime(ctx context.Context) (string, error) {
	if nvrMediaToolsAvailable() {
		return "FFmpeg and FFprobe are already installed", nil
	}
	if !packageInstallMu.TryLock() {
		return "", errors.New("package installation is already in progress")
	}
	defer packageInstallMu.Unlock()

	apt, err := exec.LookPath("apt-get")
	if err != nil {
		return "", errors.New("apt-get is unavailable")
	}
	systemdRun, err := exec.LookPath("systemd-run")
	if err != nil {
		return "", errors.New("systemd-run is unavailable")
	}

	for _, args := range [][]string{{"update"}, {"install", "-y", "ffmpeg"}} {
		commandArgs := transientPackageCommand(apt, args...)
		cmd := exec.CommandContext(ctx, systemdRun, commandArgs...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			message := strings.TrimSpace(string(output))
			if message == "" {
				message = err.Error()
			}
			return "", fmt.Errorf("install FFmpeg: %s", message)
		}
	}
	if !nvrMediaToolsAvailable() {
		return "", errors.New("FFmpeg installation completed but ffmpeg/ffprobe are unavailable")
	}
	return "FFmpeg and FFprobe installed", nil
}
