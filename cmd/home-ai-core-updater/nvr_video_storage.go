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
	"strings"
	"syscall"
	"crypto/sha256"
	"bytes"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
)

const (
	nvrStorageConfirm         = "MIGRATE_NVR_RECORDINGS"
	nvrVideoDirectory         = ".home-ai-nvr"
	nvrVideoContainerMount    = "/data/recordings"
	nvrStorageBackupContainer = "home-ai-module-nvr.module-pre-video-storage"
)

type nvrContainerMetadata struct {
	ID     string `json:"Id"`
	Image  string `json:"Image"`
	Config struct {
		Image  string `json:"Image"`
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
	State struct {
		Running bool `json:"Running"`
	} `json:"State"`
	Mounts []struct {
		Type        string `json:"Type"`
		Source      string `json:"Source"`
		Destination string `json:"Destination"`
	} `json:"Mounts"`
}

func safeNVRDiskMount(ctx context.Context, mountpoint, uuid string) error {
	root := filepath.Clean(mountpoint)
	if !strings.HasPrefix(root, "/mnt/home-ai-core/") ||
		root == "/mnt/home-ai-core" || uuid == "" ||
		strings.ContainsAny(uuid, "/\\\r\n\x00") {
		return errors.New("select a mounted NVR partition under /mnt/home-ai-core")
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("NVR disk mount is unavailable or a symlink")
	}
	path, err := filepath.EvalSymlinks(root)
	if err != nil || path != root {
		return errors.New("NVR disk mount is not canonical")
	}
	// A missing removable disk must never silently write video to the system
	// filesystem beneath its old mount directory.
	for _, spec := range [][]string{
		{"--target", root, "-n", "-o", "TARGET"},
		{"--target", root, "-n", "-o", "UUID"},
	} {
		output, err := exec.CommandContext(ctx, "findmnt", spec...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("verify NVR disk mount: %s", commandError(output, err))
		}
		value := strings.TrimSpace(string(output))
		if spec[len(spec)-1] == "TARGET" && value != root {
			return errors.New("NVR path is not the root of a mounted disk")
		}
		if spec[len(spec)-1] == "UUID" && value != uuid {
			return errors.New("mounted NVR filesystem UUID differs from selected disk")
		}
	}
	return nil
}

func inspectNVRVideoContainer(ctx context.Context, dockerPath string) (nvrContainerMetadata, error) {
	output, err := exec.CommandContext(ctx, dockerPath, "inspect", "--format", "{{json .}}",
		moduleContainerName("nvr.module")).CombinedOutput()
	if err != nil {
		return nvrContainerMetadata{}, fmt.Errorf("inspect existing NVR container: %s", commandError(output, err))
	}
	var info nvrContainerMetadata
	if err := json.Unmarshal(output, &info); err != nil {
		return info, fmt.Errorf("decode NVR Docker metadata: %w", err)
	}
	if info.ID == "" || !localImageIDPattern.MatchString(info.Image) ||
		info.Config.Labels["home-ai.managed"] != "true" ||
		info.Config.Labels["home-ai.module.id"] != "nvr.module" {
		return info, errors.New("refusing to replace unmanaged or untrusted NVR image")
	}
	expected := filepath.Join(moduleDataRoot, "nvr.module")
	hasData := false
	for _, m := range info.Mounts {
		switch m.Destination {
		case "/data":
			hasData = m.Type == "bind" && m.Source == expected
		case nvrVideoContainerMount:
			// An already attached disk can be verified separately.
		default:
			// Future NVR images with extra privileged mounts are not
			// eligible for implicit replacement by this migration.
			return info, errors.New("NVR has unexpected container mounts; manual review required")
		}
	}
	if !hasData {
		return info, errors.New("NVR data mount differs from managed persistent storage")
	}
	return info, nil
}

func nvrVideoMountSource(info nvrContainerMetadata) string {
	for _, m := range info.Mounts {
		if m.Destination == nvrVideoContainerMount && m.Type == "bind" {
			return m.Source
		}
	}
	return ""
}

func ensureNVROwnedDirectory(path string, uid, gid int) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(path, 0o700); err != nil {
			return err
		}
		info, err = os.Lstat(path)
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("NVR destination directory is unsafe")
	}
	if err := os.Chown(path, uid, gid); err != nil {
		return err
	}
	return os.Chmod(path, 0o700)
}

// Copy only regular archival files and directories. Original files are kept
// untouched as a migration backup. Existing different destination files are
// never overwritten, and partial copies are never treated as complete.
func copyNVRRecordings(ctx context.Context, source, destination string, uid, gid int) error {
	info, err := os.Lstat(source)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("NVR source archive directory is unsafe")
	}
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		target := filepath.Join(destination, relative)
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("NVR archive contains symlinks; refuse migration")
		}
		if entry.IsDir() {
			return ensureNVROwnedDirectory(target, uid, gid)
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			return errors.New("NVR archive contains a nonregular file")
		}
		if oldInfo, err := os.Lstat(target); err == nil {
			if oldInfo.Mode().IsRegular() && oldInfo.Size() == info.Size() {
				same, compareErr := equalNVRFileContents(path, target)
				if compareErr != nil {
					return compareErr
				}
				if same {
					return nil
				}
			}
			return errors.New("NVR destination already has a conflicting recording")
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		defer input.Close()
		output, err := os.CreateTemp(filepath.Dir(target), ".nvr-copy-*")
		if err != nil {
			return err
		}
		tmp := output.Name()
		defer os.Remove(tmp)
		if _, err := io.Copy(output, input); err != nil {
			_ = output.Close()
			return err
		}
		if err := output.Sync(); err != nil {
			_ = output.Close()
			return err
		}
		if err := output.Close(); err != nil {
			return err
		}
		if err := os.Chown(tmp, uid, gid); err != nil {
			return err
		}
		if err := os.Chmod(tmp, 0o600); err != nil {
			return err
		}
		return os.Rename(tmp, target)
	})
}

func nvrDockerCommand(ctx context.Context, docker, action string, args ...string) error {
	cmdArgs := append([]string{action}, args...)
	output, err := exec.CommandContext(ctx, docker, cmdArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("Docker %s: %s", action, commandError(output, err))
	}
	return nil
}

func activateNVRVideoStorage(ctx context.Context, req updaterhelper.Request, uid, gid int) (string, error) {
	if !moduleOperationMu.TryLock() {
		return "", errors.New("another Docker module operation is in progress")
	}
	defer moduleOperationMu.Unlock()
	if req.ModuleID != "nvr.module" || req.Confirm != nvrStorageConfirm {
		return "", errors.New("explicit NVR migration confirmation is required")
	}
	if err := safeNVRDiskMount(ctx, req.Mountpoint, req.FilesystemUUID); err != nil {
		return "", err
	}
	docker, err := exec.LookPath("docker")
	if err != nil {
		return "", err
	}
	info, err := inspectNVRVideoContainer(ctx, docker)
	if err != nil {
		return "", err
	}
	destinationParent := filepath.Join(req.Mountpoint, nvrVideoDirectory)
	if err := ensureNVROwnedDirectory(destinationParent, uid, gid); err != nil {
		return "", err
	}
	destination := filepath.Join(destinationParent, "recordings")
	if err := ensureNVROwnedDirectory(destination, uid, gid); err != nil {
		return "", err
	}
	if active := nvrVideoMountSource(info); active != "" {
		if active == destination {
			return destination, nil // idempotent re-acknowledgement
		}
		return "", errors.New("NVR is already attached to another video disk")
	}
	oldName := moduleContainerName("nvr.module")
	output, err := exec.CommandContext(ctx, docker, "inspect", nvrStorageBackupContainer).CombinedOutput()
	if err == nil || !strings.Contains(strings.ToLower(string(output)), "no such") {
		return "", errors.New("an earlier NVR storage migration needs recovery; refusing another attempt")
	}
	// Stop recording so files cannot change during the copy.
	if err := nvrDockerCommand(ctx, docker, "stop", "-t", "30", oldName); err != nil {
		return "", err
	}
	oldStillNamed := true
	defer func() {
		// If the operation fails before renaming, restore the old recorder.
		if oldStillNamed {
			rollback, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			_ = nvrDockerCommand(rollback, docker, "start", oldName)
		}
	}()
	source := filepath.Join(moduleDataRoot, "nvr.module", "recordings")
	if err := copyNVRRecordings(ctx, source, destination, uid, gid); err != nil {
		return "", fmt.Errorf("copy NVR recordings without deleting originals: %w", err)
	}
	if err := safeNVRDiskMount(ctx, req.Mountpoint, req.FilesystemUUID); err != nil {
		return "", err
	}
	if err := nvrDockerCommand(ctx, docker, "rename", oldName, nvrStorageBackupContainer); err != nil {
		return "", err
	}
	oldStillNamed = false
	switched := false
	defer func() {
		if !switched {
			rollback, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			_ = nvrDockerCommand(rollback, docker, "rm", "-f", oldName)
			_ = nvrDockerCommand(rollback, docker, "rename", nvrStorageBackupContainer, oldName)
			_ = nvrDockerCommand(rollback, docker, "start", oldName)
		}
	}()
	dataRoot := filepath.Join(moduleDataRoot, "nvr.module")
	args := []string{
		"--name", oldName, "--label", "home-ai.managed=true",
		"--label", "home-ai.module.id=nvr.module",
		"--label", "home-ai.nvr.video-storage=" + req.FilesystemUUID,
		"--network", dockerNetworkName,
		"--user", fmt.Sprintf("%d:%d", uid, gid), "--restart", "unless-stopped",
		"--cap-drop", "ALL", "--security-opt", "no-new-privileges",
		"--pids-limit", "512", "--read-only",
		"--tmpfs", "/tmp:rw,noexec,nosuid,size=64m",
		"--mount", "type=bind,src=" + dataRoot + ",dst=/data",
		"--mount", "type=bind,src=" + destination + ",dst=/data/recordings",
		validatedNVRImageReference(info),
	}
	if err := nvrDockerCommand(ctx, docker, "create", args...); err != nil {
		return "", err
	}
	if err := nvrDockerCommand(ctx, docker, "start", oldName); err != nil {
		return "", err
	}
	healthCtx, healthCancel := context.WithTimeout(ctx, 35*time.Second)
	defer healthCancel()
	healthy := false
	for !healthy {
		// The NVR Python runtime is part of the existing pinned image.
		cmd := exec.CommandContext(healthCtx, docker, "exec", oldName, "python3", "-c",
			"import urllib.request; urllib.request.urlopen('http://127.0.0.1:8090/api/nvr/status',timeout=2).read()")
		if cmd.Run() == nil {
			healthy = true
			break
		}
		if healthCtx.Err() != nil {
			return "", errors.New("NVR failed health check after video storage migration")
		}
		select {
		case <-healthCtx.Done():
			return "", errors.New("NVR health check timed out after storage migration")
		case <-time.After(time.Second):
		}
	}
	if err := saveNVRVideoMountConfig(nvrVideoMountConfig{
		Mountpoint: req.Mountpoint, UUID: req.FilesystemUUID,
	}); err != nil {
		return "", fmt.Errorf("persist trusted NVR mount configuration: %w", err)
	}
	switched = true
	// We do not delete the old /data/recordings directory: it is the
	// migration backup, retained until the operator verifies recordings.
	cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := nvrDockerCommand(cleanup, docker, "rm", "-f", nvrStorageBackupContainer); err != nil {
		return destination, fmt.Errorf("NVR active on new disk but previous container cleanup failed: %w", err)
	}
	return destination, nil
}

func nvrVideoStorageMounted(ctx context.Context, uuid string) (bool, string) {
	docker, err := exec.LookPath("docker")
	if err != nil {
		return false, ""
	}
	info, err := inspectNVRVideoContainer(ctx, docker)
	if err != nil {
		return false, ""
	}
	if info.Config.Labels["home-ai.nvr.video-storage"] != uuid {
		return false, ""
	}
	source := nvrVideoMountSource(info)
	if source == "" {
		return false, ""
	}
	if err := safeNVRDiskMount(ctx, filepath.Dir(filepath.Dir(source)), uuid); err != nil {
		return false, ""
	}
	return true, source
}

func nvrDiskFreeBytes(path string) (uint64, error) {
	var fs syscall.Statfs_t
	if err := syscall.Statfs(path, &fs); err != nil {
		return 0, err
	}
	return fs.Bavail * uint64(fs.Bsize), nil
}

type nvrVideoMountConfig struct {
	Mountpoint string `json:"mountpoint"`
	UUID       string `json:"filesystem_uuid"`
}

// Helper-owned, root-only state: never store the trusted mount target inside
// /data, because modules can write that directory.
var nvrVideoMountConfigPath = filepath.Join(trustedUpdateRoot, "nvr-video-storage.json")

func readNVRVideoMountConfig() (nvrVideoMountConfig, bool, error) {
	info, err := os.Lstat(nvrVideoMountConfigPath)
	if errors.Is(err, os.ErrNotExist) {
		return nvrVideoMountConfig{}, false, nil
	}
	if err != nil {
		return nvrVideoMountConfig{}, false, err
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !ok || owner.Uid != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return nvrVideoMountConfig{}, false, errors.New("NVR mount config is not a root-owned private file")
	}
	data, err := os.ReadFile(nvrVideoMountConfigPath)
	if err != nil || len(data) > 2048 {
		return nvrVideoMountConfig{}, false, errors.New("invalid NVR mount config")
	}
	var config nvrVideoMountConfig
	if err := json.Unmarshal(data, &config); err != nil ||
		config.Mountpoint == "" || config.UUID == "" {
		return nvrVideoMountConfig{}, false, errors.New("invalid NVR mount config")
	}
	return config, true, nil
}

func saveNVRVideoMountConfig(config nvrVideoMountConfig) error {
	if config.Mountpoint == "" || config.UUID == "" {
		return errors.New("invalid NVR mount config")
	}
	previous, existing, err := readNVRVideoMountConfig()
	if err != nil {
		return err
	}
	if existing && previous != config {
		return errors.New("NVR already has another trusted video disk assignment")
	}
	payload, err := json.Marshal(config)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(nvrVideoMountConfigPath), ".nvr-storage-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(payload); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), nvrVideoMountConfigPath)
}

func validatedNVRImageReference(info nvrContainerMetadata) string {
	if imageDigestPattern.MatchString(info.Config.Image) {
		return info.Config.Image
	}
	// The legacy pinned-source recovery uses the immutable local image ID.
	return info.Image
}

func equalNVRFileContents(a, b string) (bool, error) {
	left, err := os.Open(a)
	if err != nil {
		return false, err
	}
	defer left.Close()
	right, err := os.Open(b)
	if err != nil {
		return false, err
	}
	defer right.Close()
	leftHash, rightHash := sha256.New(), sha256.New()
	if _, err := io.Copy(leftHash, left); err != nil {
		return false, err
	}
	if _, err := io.Copy(rightHash, right); err != nil {
		return false, err
	}
	return bytes.Equal(leftHash.Sum(nil), rightHash.Sum(nil)), nil
}
