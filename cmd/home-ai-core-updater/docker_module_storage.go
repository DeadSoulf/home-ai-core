package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
)

const nvrVolumeMarkerFile = ".home-ai-core-nvr-storage"
const nvrRecordingDestination = "/data/recordings"

type nvrAttachedVolume struct {
	Source string
	Marker string
}

// A different filesystem must never silently receive recordings when the
// assigned disk is absent at boot. Images >= 1.1.3 check this marker on start.
func validateNVRRecordingSource(ctx context.Context, device, mountpoint string) (string, string, error) {
	if device == "" || mountpoint == "" || !strings.HasPrefix(device, "/dev/") {
		return "", "", errors.New("recording storage requires an existing block device and mountpoint")
	}
	mountpoint = filepath.Clean(mountpoint)
	relative, err := filepath.Rel("/mnt/home-ai-core", mountpoint)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, "../") || strings.Contains(relative, string(filepath.Separator)) {
		return "", "", errors.New("NVR storage must be mounted directly under /mnt/home-ai-core")
	}
	realPath, err := filepath.EvalSymlinks(mountpoint)
	if err != nil || realPath != mountpoint {
		return "", "", errors.New("NVR mountpoint must not be a symlink")
	}
	output, err := hostMountCommand(ctx, "/usr/bin/findmnt", "--noheadings", "--output", "TARGET,SOURCE,FSTYPE,UUID", "--target", mountpoint).CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("verify mounted NVR filesystem: %s", commandError(output, err))
	}
	fields := strings.Fields(string(output))
	if len(fields) != 4 || fields[0] != mountpoint || fields[1] != device ||
		(fields[2] != "ext4" && fields[2] != "xfs") || fields[3] == "-" {
		return "", "", errors.New("selected NVR storage must be a mounted ext4/xfs partition with a stable filesystem UUID")
	}
	identifier := sha256.Sum256([]byte("home-ai-core-nvr-storage-v1:" + fields[3]))
	return filepath.Join(mountpoint, "nvr-recordings"), hex.EncodeToString(identifier[:]), nil
}

func nvrRootHasRecordings(dataDir string) (bool, error) {
	// Refuse any pre-existing files, including orphaned recordings that may
	// no longer have SQLite rows. Even a single archive must be migrated
	// explicitly; never obscure it by a new bind mount.
	root := filepath.Join(dataDir, "recordings")
	items, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return true, err
	}
	if len(items) == 0 {
		return false, nil
	}
	return true, nil
}

func inspectNVRRecordingMount(ctx context.Context, dockerPath string) (string, error) {
	output, err := exec.CommandContext(ctx, dockerPath, "inspect", "--format", "{{json .}}", moduleContainerName("nvr.module")).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("inspect NVR recording mount: %s", commandError(output, err))
	}
	var container struct {
		Config struct {
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
		Mounts []struct {
			Type string `json:"Type"`
			Source string `json:"Source"`
			Destination string `json:"Destination"`
		} `json:"Mounts"`
	}
	if err := json.Unmarshal(output, &container); err != nil {
		return "", err
	}
	if container.Config.Labels["home-ai.managed"] != "true" || container.Config.Labels["home-ai.module.id"] != "nvr.module" {
		return "", errors.New("unmanaged NVR container")
	}
	for _, m := range container.Mounts {
		if m.Destination == nvrRecordingDestination {
			if m.Type != "bind" || !strings.HasPrefix(m.Source, "/mnt/home-ai-core/") {
				return "", errors.New("untrusted NVR recording volume")
			}
			return m.Source, nil
		}
	}
	return "", nil
}

func attachNVRRecordingStorage(ctx context.Context, request updaterhelper.Request, serviceUID, serviceGID int) (moduleContainerStatus, string, error) {
	if !moduleOperationMu.TryLock() {
		return moduleContainerStatus{}, "", errors.New("another module operation is in progress")
	}
	defer moduleOperationMu.Unlock()
	if request.ModuleID != "nvr.module" {
		return moduleContainerStatus{}, "", errors.New("unsupported storage attachment target")
	}
	if runtime := inspectDockerRuntime(ctx); !runtime.Active || !runtime.NetworkReady {
		return moduleContainerStatus{}, "", fmt.Errorf("Docker engine is unavailable: %s", runtime.Error)
	}
	source, marker, err := validateNVRRecordingSource(ctx, request.Device, request.Mountpoint)
	if err != nil {
		return moduleContainerStatus{}, "", err
	}
	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		return moduleContainerStatus{}, "", err
	}
	name := moduleContainerName("nvr.module")
	exists, managed, err := inspectManagedContainer(ctx, dockerPath, name, "nvr.module")
	if err != nil || !exists || !managed {
		return moduleContainerStatus{}, "", errors.New("managed NVR container is not installed")
	}
	if attached, err := inspectNVRRecordingMount(ctx, dockerPath); err != nil {
		return moduleContainerStatus{}, "", err
	} else if attached == source {
		status, err := inspectModuleContainer(ctx, dockerPath, name)
		return status, "NVR recording storage is already attached", err
	} else if attached != "" {
		return moduleContainerStatus{}, "", errors.New("NVR has an existing recording volume; migration/detach must be performed separately")
	}
	old, err := inspectNVRStorageContainer(ctx, dockerPath, name)
	if err != nil {
		return moduleContainerStatus{}, "", err
	}
	if !old.Running || !old.Supported {
		return moduleContainerStatus{}, "", errors.New("NVR must be running on version 1.1.3 or newer to safely attach recording storage")
	}
	if hasFiles, err := nvrRootHasRecordings(filepath.Join(moduleDataRoot, "nvr.module")); err != nil {
		return moduleContainerStatus{}, "", err
	} else if hasFiles {
		return moduleContainerStatus{}, "", errors.New("existing NVR recordings detected: attach blocked until recordings are migrated to avoid hiding video data")
	}
	if items, err := os.ReadDir(source); err == nil {
		for _, item := range items {
			if item.Name() != nvrVolumeMarkerFile {
				return moduleContainerStatus{}, "", errors.New("selected NVR recording directory is not empty; cannot overwrite existing files")
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return moduleContainerStatus{}, "", err
	}
	if err := os.MkdirAll(source, 0o700); err != nil {
		return moduleContainerStatus{}, "", err
	}
	if err := os.Chown(source, serviceUID, serviceGID); err != nil {
		return moduleContainerStatus{}, "", err
	}
	markerPath := filepath.Join(source, nvrVolumeMarkerFile)
	if data, err := os.ReadFile(markerPath); err == nil {
		if strings.TrimSpace(string(data)) != marker {
			return moduleContainerStatus{}, "", errors.New("existing NVR storage marker does not match selected filesystem")
		}
	} else if errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(markerPath, []byte(marker+"\n"), 0o600); err != nil {
			return moduleContainerStatus{}, "", err
		}
	} else {
		return moduleContainerStatus{}, "", err
	}
	if err := os.Chown(markerPath, serviceUID, serviceGID); err != nil {
		return moduleContainerStatus{}, "", err
	}
	const suffix = "-storage-backup"
	backup := name + suffix
	if found, _, _ := inspectManagedContainer(ctx, dockerPath, backup, "nvr.module"); found {
		return moduleContainerStatus{}, "", errors.New("previous NVR storage transaction needs recovery; backup container still exists")
	}

	cmd := func(op string, args ...string) error {
		all := append([]string{op}, args...)
		output, err := exec.CommandContext(ctx, dockerPath, all...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("docker %s: %s", op, commandError(output, err))
		}
		return nil
	}
	if err := cmd("stop", name); err != nil {
		return moduleContainerStatus{}, "", err
	}
	if err := cmd("rename", name, backup); err != nil {
		_ = exec.Command(dockerPath, "start", name).Run()
		return moduleContainerStatus{}, "", err
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		// Roll back even if the UI request was cancelled.
		recovery, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		_ = exec.CommandContext(recovery, dockerPath, "rm", "-f", name).Run()
		if output, err := exec.CommandContext(recovery, dockerPath, "rename", backup, name).CombinedOutput(); err != nil {
			// Leave the stopped backup intact for manual inspection.
			_ = output
			return
		}
		_ = exec.CommandContext(recovery, dockerPath, "start", name).Run()
	}()
	args := []string{
		"create", "--name", name,
		"--label", "home-ai.managed=true", "--label", "home-ai.module.id=nvr.module",
		"--label", "home-ai.module.nvr-recordings-root="+source,
		"--network", dockerNetworkName,
		"--user", fmt.Sprintf("%d:%d", serviceUID, serviceGID),
		"--restart", "unless-stopped", "--cap-drop", "ALL",
		"--security-opt", "no-new-privileges", "--pids-limit", "512",
		"--read-only", "--tmpfs", "/tmp:rw,noexec,nosuid,size=64m",
		"--env", "NVR_STORAGE_MARKER="+marker,
		"--mount", "type=bind,src="+filepath.Join(moduleDataRoot, "nvr.module")+",dst=/data",
		"--mount", "type=bind,src="+source+",dst="+nvrRecordingDestination,
	}
	if old.RequestedImage != "" {
		args = append(args, "--label", "home-ai.module.requested-image="+old.RequestedImage)
	}
	if old.SourceRevision != "" {
		args = append(args, "--label", "home-ai.module.source-revision="+old.SourceRevision)
	}
	args = append(args, old.Image)
	output, err := exec.CommandContext(ctx, dockerPath, args...).CombinedOutput()
	if err != nil {
		return moduleContainerStatus{}, "", fmt.Errorf("create NVR with recording volume: %s", commandError(output, err))
	}
	if err := cmd("start", name); err != nil {
		return moduleContainerStatus{}, "", err
	}
	if err := waitNVRHealthy(ctx, dockerPath, name); err != nil {
		return moduleContainerStatus{}, "", err
	}
	committed = true
	// The replacement is healthy. Never discard its data even if the old
	// stopped backup cannot be removed now.
	_ = exec.CommandContext(ctx, dockerPath, "rm", "-f", backup).Run()
	status, err := inspectModuleContainer(ctx, dockerPath, name)
	return status, "NVR recording volume attached and healthy", err
}

type nvrStorageContainerInfo struct {
	Image string
	Running bool
	Supported bool
	RequestedImage string
	SourceRevision string
}

func inspectNVRStorageContainer(ctx context.Context, dockerPath, name string) (nvrStorageContainerInfo, error) {
	output, err := exec.CommandContext(ctx, dockerPath, "inspect", "--format", "{{json .}}", name).CombinedOutput()
	if err != nil { return nvrStorageContainerInfo{}, fmt.Errorf("inspect NVR: %s", commandError(output, err)) }
	var data struct {
		State struct { Running bool `json:"Running"` } `json:"State"`
		Config struct {
			Image string `json:"Image"`
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
	}
	if err := json.Unmarshal(output, &data); err != nil { return nvrStorageContainerInfo{}, err }
	labels := data.Config.Labels
	if labels["home-ai.managed"] != "true" || labels["home-ai.module.id"] != "nvr.module" { return nvrStorageContainerInfo{}, errors.New("unmanaged NVR container") }
	version := labels["org.opencontainers.image.version"]
	// Only marker-aware NVR images can be attached; old 1.1.2 images
	// silently fall back to host-root storage if the partition is absent.
	supported := version == "1.1.3" || strings.HasPrefix(version,"1.1.4") ||
		strings.HasPrefix(version,"1.1.5") || strings.HasPrefix(version, "1.2.")
	if !imageDigestPattern.MatchString(data.Config.Image) && !localImageIDPattern.MatchString(data.Config.Image) {
		return nvrStorageContainerInfo{}, errors.New("NVR image is not pinned")
	}
	return nvrStorageContainerInfo{
		Image: data.Config.Image, Running: data.State.Running, Supported: supported,
		RequestedImage: labels["home-ai.module.requested-image"],
		SourceRevision: labels["home-ai.module.source-revision"],
	}, nil
}

func waitNVRHealthy(ctx context.Context, dockerPath, name string) error {
	deadline := time.Now().Add(50*time.Second)
	for time.Now().Before(deadline) {
		output, err := exec.CommandContext(ctx, dockerPath, "inspect", "--format",
			`{{.State.Status}}|{{if .State.Health}}{{.State.Health.Status}}{{end}}`, name).CombinedOutput()
		if err != nil { return fmt.Errorf("NVR health check: %s", commandError(output, err)) }
		parts := strings.Split(strings.TrimSpace(string(output)), "|")
		if len(parts) != 2 || parts[0] != "running" || parts[1] == "unhealthy" {
			return errors.New("replacement NVR exited or became unhealthy")
		}
		if parts[1] == "healthy" { return nil }
		select { case <-ctx.Done(): return ctx.Err(); case <-time.After(time.Second): }
	}
	return errors.New("replacement NVR did not become healthy; rolling back")
}
