package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
		return "", errors.New("NAS pool root must be the filesystem mount point itself")
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
