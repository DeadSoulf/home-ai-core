package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var nvrMountName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,100}$`)

type dockerNVRState struct {
	ID string `json:"Id"`
	Config struct {
		Image string `json:"Image"`
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
	State struct {
		Running bool `json:"Running"`
	} `json:"State"`
	Mounts []struct {
		Source string `json:"Source"`
		Destination string `json:"Destination"`
	} `json:"Mounts"`
}

// upgradeNVRDockerModule keeps the old container stopped as a rollback
// candidate until the new image passes health checks. It never deletes /data.
func upgradeNVRDockerModule(ctx context.Context, previousImage, newImage, requestedMount string, serviceUID, serviceGID int) (moduleContainerStatus, string, error) {
	dockerPath, err := exec.LookPath("docker")
	if err != nil { return moduleContainerStatus{}, "", errors.New("Docker CLI is unavailable") }
	const moduleID = "nvr.module"
	name := moduleContainerName(moduleID)
	original, err := inspectNVRUpgradeContainer(ctx, dockerPath, name)
	if err != nil { return moduleContainerStatus{}, "", err }
	if original.Config.Labels["home-ai.managed"] != "true" ||
		original.Config.Labels["home-ai.module.id"] != moduleID || original.ID == "" {
		return moduleContainerStatus{}, "", errors.New("refusing to upgrade unmanaged NVR container")
	}
	if !original.State.Running {
		return moduleContainerStatus{}, "", errors.New("NVR must be running before upgrade")
	}
	currentImage := original.Config.Image
	oldMatches := currentImage == previousImage ||
		original.Config.Labels["home-ai.module.requested-image"] == previousImage
	if !oldMatches && currentImage != newImage {
		return moduleContainerStatus{}, "", errors.New("NVR installed image differs from recorded immutable manifest")
	}
	currentVideoSource := ""
	for _, m := range original.Mounts {
		if m.Destination == "/data/video" {
			currentVideoSource = filepath.Clean(m.Source)
		}
	}
	videoSource := currentVideoSource
	if requestedMount != "" {
		source, err := prepareNVRVideoStorage(ctx, requestedMount, serviceUID, serviceGID)
		if err != nil { return moduleContainerStatus{}, "", err }
		if currentVideoSource != "" && currentVideoSource != source {
			return moduleContainerStatus{}, "", errors.New("cannot replace an existing video disk without verified migration")
		}
		videoSource = source
	}
	if currentImage == newImage &&
		(requestedMount == "" || currentVideoSource == videoSource) {
		return moduleContainerStatus{State: "running", ContainerID: original.ID},
			"requested NVR image and video disk are already active", nil
	}
	backupName := name + "-rollback"
	if output, err := exec.CommandContext(ctx, dockerPath, "inspect", "--format", "{{.Id}}", backupName).CombinedOutput(); err == nil && strings.TrimSpace(string(output)) != "" {
		return moduleContainerStatus{}, "", errors.New("an NVR rollback container already exists; reconcile it before upgrading")
	}
	if currentImage != newImage {
		if err := pullDockerModuleImage(ctx, dockerPath, newImage); err != nil {
			return moduleContainerStatus{}, "", fmt.Errorf("download new NVR image before stopping old container: %w", err)
		}
	}
	backupPath := filepath.Join(moduleDataRoot, moduleID)
	if output, err := exec.CommandContext(ctx, dockerPath, "stop", "--time", "20", name).CombinedOutput(); err != nil {
		return moduleContainerStatus{}, "", fmt.Errorf("stop old NVR: %s", commandError(output, err))
	}
	// Once stopped, SQLite WAL files are stable; copy only the metadata DB.
	// Video files are never copied or deleted by a module upgrade.
	dbBackup, err := snapshotNVRDatabase(backupPath)
	if err != nil {
		_, restartErr := exec.CommandContext(ctx, dockerPath, "start", name).CombinedOutput()
		return moduleContainerStatus{}, "", errors.Join(err, restartErr)
	}
	defer os.RemoveAll(dbBackup)
	if output, err := exec.CommandContext(ctx, dockerPath, "rename", name, backupName).CombinedOutput(); err != nil {
		_, restartErr := exec.CommandContext(ctx, dockerPath, "start", name).CombinedOutput()
		return moduleContainerStatus{}, "", errors.Join(
			fmt.Errorf("reserve old NVR container: %s", commandError(output, err)), restartErr,
		)
	}

	committed := false
	defer func() {
		if !committed {
			// Safe rollback is performed explicitly below. The placeholder
			// avoids any automatic deletion on a partially successful swap.
		}
	}()
	rollback := func(cause error) (moduleContainerStatus, string, error) {
		recoveryCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		var issues []error
		if output, err := exec.CommandContext(recoveryCtx, dockerPath, "rm", "-f", name).CombinedOutput(); err != nil {
			issues = append(issues, fmt.Errorf("remove failed candidate: %s", commandError(output, err)))
		}
		if err := restoreNVRDatabase(backupPath, dbBackup); err != nil {
			issues = append(issues, fmt.Errorf("restore NVR metadata: %w", err))
		}
		if output, err := exec.CommandContext(recoveryCtx, dockerPath, "rename", backupName, name).CombinedOutput(); err != nil {
			issues = append(issues, fmt.Errorf("restore previous container name: %s", commandError(output, err)))
		} else if output, err := exec.CommandContext(recoveryCtx, dockerPath, "start", name).CombinedOutput(); err != nil {
			issues = append(issues, fmt.Errorf("restart previous NVR: %s", commandError(output, err)))
		}
		return moduleContainerStatus{}, "", errors.Join(append([]error{cause}, issues...)...)
	}

	args := []string{
		"create", "--name", name,
		"--label", "home-ai.managed=true",
		"--label", "home-ai.module.id=nvr.module",
		"--label", "home-ai.module.requested-image="+newImage,
		"--network", dockerNetworkName,
		"--user", fmt.Sprintf("%d:%d", serviceUID, serviceGID),
		"--restart", "unless-stopped", "--cap-drop", "ALL",
		"--security-opt", "no-new-privileges", "--pids-limit", "512",
		"--read-only", "--tmpfs", "/tmp:rw,noexec,nosuid,size=64m",
		"--mount", "type=bind,src="+backupPath+",dst=/data",
	}
	if videoSource != "" {
		args = append(args,
			"--mount", "type=bind,src="+videoSource+",dst=/data/video",
			"--env", "NVR_RECORDINGS_ROOT=/data/video",
			"--label", "home-ai.nvr.video-source="+videoSource,
		)
	}
	args = append(args, newImage)
	output, err := exec.CommandContext(ctx, dockerPath, args...).CombinedOutput()
	if err != nil {
		return rollback(fmt.Errorf("create new NVR container: %s", commandError(output, err)))
	}
	if output, err := exec.CommandContext(ctx, dockerPath, "start", name).CombinedOutput(); err != nil {
		return rollback(fmt.Errorf("start updated NVR: %s", commandError(output, err)))
	}
	if err := waitHealthyNVR(ctx, dockerPath, name); err != nil {
		return rollback(err)
	}
	// The old container is not removed until the replacement is healthy.
	// Container removal never removes the bind-mounted recordings.
	if output, err := exec.CommandContext(ctx, dockerPath, "rm", backupName).CombinedOutput(); err != nil {
		return moduleContainerStatus{State: "running"}, "NVR upgraded, but previous stopped container remains: "+commandError(output, err), nil
	}
	committed = true
	status, err := inspectModuleContainer(ctx, dockerPath, name)
	if err != nil { return moduleContainerStatus{State: "running"}, "NVR upgraded; container inspection is delayed", nil }
	return status, "NVR upgraded with old database and recordings retained", nil
}

func inspectNVRUpgradeContainer(ctx context.Context, dockerPath, name string) (dockerNVRState, error) {
	out, err := exec.CommandContext(ctx, dockerPath, "inspect", "--format", "{{json .}}", name).CombinedOutput()
	if err != nil { return dockerNVRState{}, fmt.Errorf("inspect NVR container: %s", commandError(out, err)) }
	var state dockerNVRState
	if err := json.Unmarshal(out, &state); err != nil {
		return state, fmt.Errorf("decode NVR container: %w", err)
	}
	return state, nil
}

func waitHealthyNVR(ctx context.Context, dockerPath, name string) error {
	deadline := time.NewTimer(65 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		output, err := exec.CommandContext(ctx, dockerPath, "inspect", "--format",
			`{{.State.Status}}|{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}`, name).CombinedOutput()
		if err != nil { return fmt.Errorf("inspect updated NVR: %s", commandError(output, err)) }
		parts := strings.SplitN(strings.TrimSpace(string(output)), "|", 2)
		if len(parts) == 2 {
			if parts[0] != "running" { return fmt.Errorf("updated NVR exited: %s", parts[0]) }
			if parts[1] == "healthy" { return nil }
			if parts[1] == "unhealthy" { return errors.New("updated NVR failed Docker health check") }
			if parts[1] == "none" { return errors.New("updated NVR image lacks health check") }
		}
		select {
		case <-ctx.Done(): return ctx.Err()
		case <-deadline.C: return errors.New("updated NVR did not become healthy within 65 seconds")
		case <-ticker.C:
		}
	}
}

func prepareNVRVideoStorage(ctx context.Context, mount string, uid, gid int) (string, error) {
	mount = filepath.Clean(mount)
	if filepath.Dir(mount) != "/mnt/home-ai-core" || !nvrMountName.MatchString(filepath.Base(mount)) {
		return "", errors.New("video disk must be mounted directly under /mnt/home-ai-core")
	}
	info, err := os.Lstat(mount)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("assigned video disk mount point is unavailable or unsafe")
	}
	out, err := exec.CommandContext(ctx, "findmnt", "--noheadings", "--mountpoint", mount, "--output", "TARGET").CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != mount {
		return "", errors.New("assigned NVR storage path is not a mounted filesystem")
	}
	root := filepath.Join(mount, "nvr-video")
	if err := os.Mkdir(root, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return "", fmt.Errorf("create NVR video directory: %w", err)
	}
	stat, err := os.Lstat(root)
	if err != nil || !stat.IsDir() || stat.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("unsafe NVR video directory")
	}
	if err := os.Chown(root, uid, gid); err != nil {
		return "", fmt.Errorf("set NVR video ownership: %w", err)
	}
	if err := os.Chmod(root, 0o700); err != nil { return "", err }
	return root, nil
}

func snapshotNVRDatabase(dataDir string) (string, error) {
	dir, err := os.MkdirTemp("", "home-ai-nvr-db-backup-*")
	if err != nil { return "", err }
	if err := os.Chmod(dir, 0o700); err != nil { os.RemoveAll(dir); return "", err }
	for _, name := range []string{"nvr.db", "nvr.db-wal", "nvr.db-shm"} {
		src := filepath.Join(dataDir, name)
		info, err := os.Lstat(src)
		if errors.Is(err, os.ErrNotExist) { continue }
		if err != nil || !info.Mode().IsRegular() || info.Size() > 256<<20 {
			os.RemoveAll(dir)
			return "", fmt.Errorf("NVR SQLite metadata %s is not safely backup-able", name)
		}
		if err := copyNVRFile(src, filepath.Join(dir, name)); err != nil {
			os.RemoveAll(dir); return "", err
		}
	}
	return dir, nil
}

func restoreNVRDatabase(dataDir, snapshot string) error {
	for _, name := range []string{"nvr.db", "nvr.db-wal", "nvr.db-shm"} {
		source := filepath.Join(snapshot, name)
		if _, err := os.Stat(source); errors.Is(err, os.ErrNotExist) {
			if removeErr := os.Remove(filepath.Join(dataDir, name)); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				return removeErr
			}
			continue
		} else if err != nil { return err }
		if err := copyNVRFile(source, filepath.Join(dataDir, name)); err != nil { return err }
	}
	return nil
}

func copyNVRFile(from, to string) error {
	src, err := os.Open(from)
	if err != nil { return err }
	defer src.Close()
	dest, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil { return err }
	if _, err := io.Copy(dest, io.LimitReader(src, 256<<20+1)); err != nil { _ = dest.Close(); return err }
	if err := dest.Sync(); err != nil { _ = dest.Close(); return err }
	return dest.Close()
}
