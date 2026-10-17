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
)

// The privileged helper independently checks the device's current UUID and
// host mount namespace. The app/browser never submits a filesystem path.
var nvrStorageDevicePattern = regexp.MustCompile(`^/dev/(?:[a-zA-Z0-9_.-]+|mapper/[a-zA-Z0-9_.-]+)$`)
var nvrStorageUUIDPattern = regexp.MustCompile(`^[a-zA-Z0-9-]{4,90}$`)

func nvrRecordingSource(device, uuid string) (string, error) {
	if !nvrStorageDevicePattern.MatchString(device) || filepath.Clean(device) != device ||
		!nvrStorageUUIDPattern.MatchString(uuid) {
		return "", errors.New("invalid NVR disk or filesystem UUID")
	}
	return filepath.Join("/mnt/home-ai-core", filepath.Base(device)), nil
}

func ensureNVRRecordingDirectory(ctx context.Context, device, uuid string, serviceUID, serviceGID int) (string, error) {
	mountpoint, err := nvrRecordingSource(device, uuid)
	if err != nil {
		return "", err
	}
	systemDevice, err := samePhysicalDiskAsRoot(ctx, device)
	if err != nil {
		return "", fmt.Errorf("inspect NVR disk ownership: %w", err)
	}
	if systemDevice {
		return "", errors.New("system/root disk cannot be assigned as NVR recordings storage")
	}
	output, err := exec.CommandContext(ctx, "/usr/sbin/blkid", "-s", "UUID", "-o", "value", "--", device).CombinedOutput()
	if err != nil || strings.TrimSpace(string(output)) != uuid {
		return "", errors.New("NVR recordings disk UUID does not match the assigned filesystem")
	}
	output, err = exec.CommandContext(ctx, "/usr/sbin/blkid", "-s", "TYPE", "-o", "value", "--", device).CombinedOutput()
	if err != nil || (strings.TrimSpace(string(output)) != "ext4" && strings.TrimSpace(string(output)) != "xfs") {
		return "", errors.New("NVR recordings disk must use a writable ext4 or xfs filesystem")
	}
	targets, err := mountedTargets(ctx, device)
	if err != nil {
		return "", err
	}
	if !mountTargetPresent(targets, mountpoint) {
		return "", fmt.Errorf("NVR disk must first be mounted at %s in the Files storage page", mountpoint)
	}

	// The recording directory is created ONLY on the already-mounted,
	// UUID-verified filesystem. If the disk disappears before reboot, the
	// Docker --mount source vanishes, so NVR fails closed instead of
	// silently writing video to the Core system partition.
	for _, dir := range []string{
		filepath.Join(mountpoint, "home-ai-nvr"),
		filepath.Join(mountpoint, "home-ai-nvr", "recordings"),
	} {
		info, err := os.Lstat(dir)
		switch {
		case errors.Is(err, os.ErrNotExist):
			if err := os.Mkdir(dir, 0o750); err != nil {
				return "", fmt.Errorf("create NVR video directory: %w", err)
			}
		case err != nil:
			return "", err
		case !info.IsDir() || info.Mode()&os.ModeSymlink != 0:
			return "", errors.New("NVR recording directory is not a regular directory")
		}
	}
	parent := filepath.Join(mountpoint, "home-ai-nvr")
	root := filepath.Join(parent, "recordings")
	for _, dir := range []string{parent, root} {
		if err := os.Chown(dir, serviceUID, serviceGID); err != nil {
			return "", err
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			return "", err
		}
	}
	return root, nil
}
