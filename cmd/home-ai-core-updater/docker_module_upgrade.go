package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Upgrade replaces a managed container by a pinned candidate and restores the
// original on failed start/health. Module volume data is never removed or
// formatted by this operation. The registry is committed separately by Core.
func upgradeDockerModule(ctx context.Context, id, previousImage, nextImage string, uid, gid int) (moduleContainerStatus, string, error) {
	if !moduleOperationMu.TryLock() { return moduleContainerStatus{}, "", errors.New("another Docker module operation is in progress") }
	defer moduleOperationMu.Unlock()
	if !moduleIDPattern.MatchString(id) || !imageDigestPattern.MatchString(previousImage) ||
		!imageDigestPattern.MatchString(nextImage) || previousImage == nextImage {
		return moduleContainerStatus{}, "", errors.New("invalid immutable module upgrade images")
	}
	runtimeStatus := inspectDockerRuntime(ctx)
	if !runtimeStatus.Active || !runtimeStatus.NetworkReady {
		return moduleContainerStatus{}, "", errors.New("Docker module network is not ready")
	}
	dockerPath, err := exec.LookPath("docker")
	if err != nil { return moduleContainerStatus{}, "", err }
	name := moduleContainerName(id)
	exists, managed, err := inspectManagedContainer(ctx, dockerPath, name, id)
	if err != nil || !exists || !managed { return moduleContainerStatus{}, "", errors.New("module container is missing or unmanaged") }
	previous, err := inspectUpgradeContainer(ctx, dockerPath, name, id)
	if err != nil { return moduleContainerStatus{}, "", err }
	if previous.ConfigImage == nextImage {
		if !previous.Running {
			return moduleContainerStatus{}, "", errors.New("upgraded container is not running; repair before retrying")
		}
		status, err := inspectModuleContainer(ctx, dockerPath, name)
		return status, "candidate module was already upgraded successfully", err
	}
	if previous.ConfigImage != previousImage &&
		!(previousImage == nvrLegacyImage && previous.RequestedImage == previousImage &&
		  localImageIDPattern.MatchString(previous.ConfigImage)) {
		return moduleContainerStatus{}, "", errors.New("module image changed unexpectedly; refusing to overwrite container")
	}
	if !previous.Running {
		return moduleContainerStatus{}, "", errors.New("module must be enabled and running before upgrade")
	}
	if err := pullDockerModuleImage(ctx, dockerPath, nextImage); err != nil {
		return moduleContainerStatus{}, "", fmt.Errorf("download new module image: %w", err)
	}
	attached := previous.RecordingSource
	if attached != "" {
		if id != "nvr.module" || previous.StorageMarker == "" ||
			!strings.HasPrefix(attached, "/mnt/home-ai-core/") {
			return moduleContainerStatus{}, "", errors.New("unsupported module recording volume")
		}
		// New images must know how to fail closed without the assigned disk.
		info, err := exec.CommandContext(ctx, dockerPath, "image", "inspect", "--format",
			`{{index .Config.Labels "org.opencontainers.image.version"}}`, nextImage).CombinedOutput()
		if err != nil || !markerAwareNVRVersion(strings.TrimSpace(string(info))) {
			return moduleContainerStatus{}, "", errors.New("new NVR image does not support fail-closed recording disks")
		}
		markerBytes, err := os.ReadFile(filepath.Join(attached, nvrVolumeMarkerFile))
		if err != nil || strings.TrimSpace(string(markerBytes)) != previous.StorageMarker {
			return moduleContainerStatus{}, "", errors.New("assigned NVR disk is absent or its marker changed")
		}
	}
	backup := name + "-upgrade-backup"
	if exists, _, _ := inspectManagedContainer(ctx, dockerPath, backup, id); exists {
		return moduleContainerStatus{}, "", errors.New("prior module upgrade backup still exists; inspect before retry")
	}
	run := func(ctx context.Context, command string, args ...string) error {
		output, err := exec.CommandContext(ctx, dockerPath, append([]string{command}, args...)...).CombinedOutput()
		if err != nil { return fmt.Errorf("docker %s: %s", command, commandError(output, err)) }
		return nil
	}
	if err := run(ctx, "stop", name); err != nil { return moduleContainerStatus{}, "", err }
	if err := run(ctx, "rename", name, backup); err != nil {
		recoverCtx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		_ = run(recoverCtx, "start", name)
		return moduleContainerStatus{}, "", err
	}
	success := false
	defer func() {
		if success { return }
		recovery, cancel := context.WithTimeout(context.Background(), 75*time.Second)
		defer cancel()
		_ = run(recovery, "rm", "-f", name)
		if run(recovery, "rename", backup, name) == nil {
			_ = run(recovery, "start", name)
		}
	}()
	args := []string{
		"--name", name,
		"--label", "home-ai.managed=true",
		"--label", "home-ai.module.id="+id,
		"--network", dockerNetworkName,
		"--user", fmt.Sprintf("%d:%d", uid, gid),
		"--restart", "unless-stopped",
		"--cap-drop", "ALL", "--security-opt", "no-new-privileges",
		"--pids-limit", "512", "--read-only",
		"--tmpfs", "/tmp:rw,noexec,nosuid,size=64m",
		"--mount", "type=bind,src="+filepath.Join(moduleDataRoot,id)+",dst=/data",
	}
	if attached != "" {
		args = append(args,
			"--label", "home-ai.module.nvr-recordings-root="+attached,
			"--env", "NVR_STORAGE_MARKER="+previous.StorageMarker,
			"--mount", "type=bind,src="+attached+",dst=/data/recordings",
		)
	}
	args = append(args, nextImage)
	if err := run(ctx, "create", args...); err != nil {
		return moduleContainerStatus{}, "", err
	}
	if err := run(ctx, "start", name); err != nil {
		return moduleContainerStatus{}, "", err
	}
	if err := waitModuleUpgradeHealthy(ctx, dockerPath, name); err != nil {
		return moduleContainerStatus{}, "", err
	}
	success = true
	// If backup cleanup fails, preserve the stopped old image for diagnosis.
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = run(cleanupCtx, "rm", "-f", backup)
	status, err := inspectModuleContainer(ctx, dockerPath, name)
	return status, "module upgraded and healthy; old data preserved", err
}

type upgradeContainerInfo struct {
	ConfigImage string
	RequestedImage string
	RecordingSource string
	StorageMarker string
	Running bool
}
func inspectUpgradeContainer(ctx context.Context, dockerPath, name, expectedID string) (upgradeContainerInfo, error) {
	output, err := exec.CommandContext(ctx, dockerPath, "inspect", "--format", "{{json .}}", name).CombinedOutput()
	if err != nil { return upgradeContainerInfo{}, fmt.Errorf("inspect module upgrade candidate: %s", commandError(output, err)) }
	var item struct {
		Config struct {
			Image string `json:"Image"`
			Labels map[string]string `json:"Labels"`
			Env []string `json:"Env"`
		} `json:"Config"`
		State struct { Running bool `json:"Running"` } `json:"State"`
		Mounts []struct {
			Type string `json:"Type"`
			Source string `json:"Source"`
			Destination string `json:"Destination"`
		} `json:"Mounts"`
	}
	if err := json.Unmarshal(output, &item); err != nil { return upgradeContainerInfo{}, err }
	if item.Config.Labels["home-ai.managed"] != "true" || item.Config.Labels["home-ai.module.id"] != expectedID {
		return upgradeContainerInfo{}, errors.New("refusing to upgrade an unmanaged module container")
	}
	info := upgradeContainerInfo{ConfigImage:item.Config.Image,
		RequestedImage:item.Config.Labels["home-ai.module.requested-image"],
		Running:item.State.Running}
	if expectedID=="nvr.module" {
		for _, mount := range item.Mounts {
			if mount.Destination=="/data/recordings" {
				if mount.Type!="bind" { return upgradeContainerInfo{}, errors.New("untrusted recording mount type") }
				info.RecordingSource=mount.Source
			}
		}
		for _, env := range item.Config.Env {
			if strings.HasPrefix(env,"NVR_STORAGE_MARKER=") {
				info.StorageMarker=strings.TrimPrefix(env,"NVR_STORAGE_MARKER=")
			}
		}
	}
	return info,nil
}
func markerAwareNVRVersion(version string) bool {
	if !strings.HasPrefix(version, "1.") { return false }
	parts := strings.Split(version, ".")
	if len(parts)!=3 { return false }
	// Current release has marker support starting at 1.1.3.
	return parts[1] != "0" && (parts[1] != "1" || parts[2] >= "3")
}
func waitModuleUpgradeHealthy(ctx context.Context, dockerPath, name string) error {
	timeout := time.NewTimer(70*time.Second)
	defer timeout.Stop()
	var firstRunning time.Time
	for {
		output, err := exec.CommandContext(ctx,dockerPath,"inspect","--format",
			`{{.State.Status}}|{{if .State.Health}}{{.State.Health.Status}}{{end}}`,name).CombinedOutput()
		if err != nil { return fmt.Errorf("inspect upgraded module: %s",commandError(output,err)) }
		parts := strings.Split(strings.TrimSpace(string(output)),"|")
		if len(parts)!=2 || parts[0]!="running" || parts[1]=="unhealthy" {
			return errors.New("upgraded module failed to stay running/healthy")
		}
		if parts[1]=="healthy" { return nil }
		if firstRunning.IsZero() { firstRunning=time.Now() }
		if parts[1]=="" && time.Since(firstRunning)>3*time.Second { return nil }
		select {
		case <-ctx.Done():return ctx.Err()
		case <-timeout.C:return errors.New("candidate health check timed out")
		case <-time.After(time.Second):
		}
	}
}
