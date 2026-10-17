package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
	"golang.org/x/sys/unix"
)

const nasMountRoot = "/mnt/home-ai-core"

func performNASOperation(
	ctx context.Context,
	request updaterhelper.Request,
	uid, gid int,
) (string, error) {
	root, err := validateNASRoot(ctx, request.RootPath)
	if err != nil {
		return "", err
	}
	dataRoot := filepath.Join(root, ".home-ai")

	switch request.Operation {
	case "storage.nas.quota.set":
		return setNASQuota(ctx, request, uid, gid)

	case "storage.nas.prepare_pool":
		if err := ensureOwnedDir(dataRoot, uid, gid, 0o750); err != nil {
			return "", err
		}
		if err := ensureOwnedDir(filepath.Join(dataRoot, "users"), uid, gid, 0o750); err != nil {
			return "", err
		}
		if err := ensureOwnedDir(filepath.Join(dataRoot, "shared"), uid, gid, 0o750); err != nil {
			return "", err
		}
		return "NAS pool prepared", nil

	case "storage.nas.capacity_policy":
		if request.ReservePercent < 0 || request.ReservePercent > 50 {
			return "", errors.New("NAS reserve percent must be between 0 and 50")
		}
		if err := enforceNASUserQuota(ctx, root, request.ReservePercent, uid); err != nil {
			return "", err
		}
		return "NAS hard quota synchronized", nil

	case "storage.nas.prepare_folder":
		relative, err := validateNASRelativePath(request.RelativePath)
		if err != nil {
			return "", err
		}
		if err := ensureOwnedDir(dataRoot, uid, gid, 0o750); err != nil {
			return "", err
		}
		current := dataRoot
		for _, segment := range strings.Split(relative, string(filepath.Separator)) {
			current = filepath.Join(current, segment)
			if err := ensureOwnedDir(current, uid, gid, 0o750); err != nil {
				return "", err
			}
		}
		return "NAS folder prepared", nil

	default:
		return "", errors.New("unsupported NAS operation")
	}
}

func inspectNASFolderUsage(ctx context.Context, request updaterhelper.Request) (int64, error) {
	root := filepath.Clean(strings.TrimSpace(request.RootPath))
	if root == "." || !filepath.IsAbs(root) || root == nasMountRoot || !strings.HasPrefix(root, nasMountRoot+string(filepath.Separator)) {
		return 0, fmt.Errorf("NAS pool root must be a mounted filesystem under %s", nasMountRoot)
	}
	relative, err := validateNASRelativePath(request.RelativePath)
	if err != nil {
		return 0, err
	}

	mountOutput, err := hostMountCommand(ctx, "/usr/bin/findmnt", "-rn", "-T", root, "-o", "TARGET").CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("inspect NAS pool mount: %s", strings.TrimSpace(string(mountOutput)))
	}
	if target := filepath.Clean(strings.TrimSpace(string(mountOutput))); target != root {
		return 0, nasMountTargetMismatch(root, target)
	}

	folder := filepath.Join(root, ".home-ai", relative)
	output, err := hostMountCommand(
		ctx,
		"/usr/bin/du",
		"-s",
		"-B1",
		"--apparent-size",
		"--one-file-system",
		"--exclude=.home-ai-upload-*",
		"--",
		folder,
	).CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("inspect NAS folder usage: %s", strings.TrimSpace(string(output)))
	}
	used, ok := parseDUUsageOutput(output)
	if !ok {
		return 0, errors.New("inspect NAS folder usage: invalid du output")
	}
	return used, nil
}

func parseDUUsageOutput(output []byte) (int64, bool) {
	fields := strings.Fields(strings.TrimSpace(string(output)))
	if len(fields) == 0 {
		return 0, false
	}
	value, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil || value < 0 {
		return 0, false
	}
	return value, true
}

func validateNASRoot(ctx context.Context, value string) (string, error) {
	root := filepath.Clean(strings.TrimSpace(value))
	if root == "." || !filepath.IsAbs(root) {
		return "", errors.New("NAS pool root must be an absolute path")
	}
	if root == nasMountRoot || !strings.HasPrefix(root, nasMountRoot+string(filepath.Separator)) {
		return "", fmt.Errorf("NAS pool root must be a mounted filesystem under %s", nasMountRoot)
	}

	info, err := os.Lstat(root)
	if err != nil {
		return "", fmt.Errorf("inspect NAS pool root: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", errors.New("NAS pool root must be a real directory, not a symlink")
	}

	output, err := exec.CommandContext(
		ctx,
		"/usr/bin/findmnt",
		"-rn",
		"-T", root,
		"-o", "TARGET",
	).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("inspect NAS pool mount: %s", strings.TrimSpace(string(output)))
	}
	target := filepath.Clean(strings.TrimSpace(string(output)))
	if target != root {
		// Host mounts are created with nsenter, while this helper may run in
		// its own mount namespace (e.g. systemd PrivateTmp). Check both views,
		// but NEVER prepare folders if this namespace cannot see the mount:
		// such writes would land on the underlying system filesystem.
		hostOutput, hostErr := hostMountCommand(ctx, "/usr/bin/findmnt",
			"-rn", "-T", root, "-o", "TARGET").CombinedOutput()
		if hostErr == nil && filepath.Clean(strings.TrimSpace(string(hostOutput))) == root {
			return "", fmt.Errorf("NAS pool mount %s is present on the host but unavailable in the updater helper mount namespace; no data was written. Check helper mount propagation and restart the helper only after verifying storage state", root)
		}
		return "", nasMountTargetMismatch(root, target)
	}

	var stat unix.Statfs_t
	if err := unix.Statfs(root, &stat); err != nil {
		return "", fmt.Errorf("inspect NAS pool filesystem: %w", err)
	}
	if stat.Flags&unix.ST_RDONLY != 0 {
		return "", errors.New("NAS pool filesystem is read-only")
	}
	return root, nil
}

// nasMountTargetMismatch is diagnostic only. The exact mountpoint
// requirement is not relaxed: a directory under '/' or a parent mount is
// never accepted as NAS storage, even if it exists and is writable.
func nasMountTargetMismatch(root, coveringTarget string) error {
	return fmt.Errorf("NAS pool root %s must be the filesystem mount point itself (current filesystem mount covering it: %s); verify the original data device is mounted at the saved pool root before retrying", root, coveringTarget)
}

func validateNASRelativePath(value string) (string, error) {
	relative := filepath.Clean(strings.TrimSpace(value))
	if relative == "." || filepath.IsAbs(relative) || strings.HasPrefix(relative, "..") {
		return "", errors.New("invalid NAS relative path")
	}
	parts := strings.Split(relative, string(filepath.Separator))
	switch {
	case len(parts) == 2 && parts[0] == "shared":
		if !validNASID(parts[1], "nsf_") {
			return "", errors.New("invalid shared folder identity")
		}
	case len(parts) == 3 && parts[0] == "users":
		if !validNASID(parts[1], "usr_") || !validNASID(parts[2], "nsf_") {
			return "", errors.New("invalid private folder identity")
		}
	default:
		return "", errors.New("NAS folder path must be shared/<folder-id> or users/<user-id>/<folder-id>")
	}
	return relative, nil
}

func validNASID(value, prefix string) bool {
	if !strings.HasPrefix(value, prefix) || len(value) <= len(prefix) || len(value) > 96 {
		return false
	}
	for _, r := range value[len(prefix):] {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func ensureOwnedDir(path string, uid, gid int, mode os.FileMode) error {
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if err := os.Mkdir(path, mode); err != nil {
			return fmt.Errorf("create NAS directory %s: %w", path, err)
		}
	case err != nil:
		return fmt.Errorf("inspect NAS directory %s: %w", path, err)
	case info.Mode()&os.ModeSymlink != 0:
		return fmt.Errorf("refusing NAS symlink path %s", path)
	case !info.IsDir():
		return fmt.Errorf("NAS path %s is not a directory", path)
	}

	if err := os.Chown(path, uid, gid); err != nil {
		return fmt.Errorf("set NAS directory ownership %s: %w", path, err)
	}
	if err := os.Chmod(path, mode); err != nil {
		return fmt.Errorf("set NAS directory permissions %s: %w", path, err)
	}
	return nil
}
