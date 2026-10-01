package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
	"golang.org/x/sys/unix"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unsafe"
)

type projectQuota struct {
	BlockHardLimit, BlockSoftLimit, CurrentSpace  uint64
	InodeHardLimit, InodeSoftLimit, CurrentInodes uint64
	BlockTime, InodeTime                          uint64
	Valid                                         uint32
	_                                             uint32
}

type projectAttributes struct {
	Flags, ExtentSize, Nextents, ProjectID, CowExtentSize uint32
	Padding                                               [8]byte
}

const (
	quotaGetProject  = 0x80000702
	quotaSetProject  = 0x80000802
	quotaBlockLimits = 1
	fsGetProject     = 0x801c581f
	fsSetProject     = 0x401c5820
	projectInherit   = 0x200
)

func projectQuotaCall(command uintptr, device string, project uint32, quota *projectQuota) error {
	path, err := unix.BytePtrFromString(device)
	if err != nil {
		return err
	}
	_, _, errno := unix.Syscall6(unix.SYS_QUOTACTL, command, uintptr(unsafe.Pointer(path)), uintptr(project), uintptr(unsafe.Pointer(quota)), 0, 0)
	if errno != 0 {
		return errno
	}
	return nil
}

func quotaDevice(ctx context.Context, root string) (string, error) {
	output, err := exec.CommandContext(ctx, "/usr/bin/findmnt", "-rn", "-T", root, "-o", "SOURCE,FSTYPE,OPTIONS").Output()
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(output))
	if len(fields) != 3 || (fields[1] != "ext4" && fields[1] != "xfs") || !strings.HasPrefix(fields[0], "/dev/") {
		return "", errors.New("hard quotas require ext4 or XFS mounted with project quota support")
	}
	enforced := false
	for _, option := range strings.Split(fields[2], ",") {
		if option == "prjquota" || option == "pquota" {
			enforced = true
		}
		if option == "pqnoenforce" || option == "noquota" {
			return "", errors.New("filesystem project quota enforcement is disabled")
		}
	}
	if !enforced {
		return "", errors.New("filesystem must be mounted with enforced prjquota")
	}
	return fields[0], nil
}

func inspectNASQuota(ctx context.Context, request updaterhelper.Request) (projectQuota, error) {
	root, err := validateNASRoot(ctx, request.RootPath)
	if err != nil {
		return projectQuota{}, err
	}
	if request.ProjectID < 1000000 {
		return projectQuota{}, errors.New("invalid Home-AI quota project")
	}
	device, err := quotaDevice(ctx, root)
	if err != nil {
		return projectQuota{}, err
	}
	var quota projectQuota
	if err := projectQuotaCall(quotaGetProject, device, request.ProjectID, &quota); err != nil {
		return projectQuota{}, fmt.Errorf("project quotas are unavailable; enable prjquota on this filesystem: %w", err)
	}
	return quota, nil
}

func setNASQuota(ctx context.Context, request updaterhelper.Request, uid, gid int) (string, error) {
	quota, err := inspectNASQuota(ctx, request)
	if err != nil {
		return "", err
	}
	if request.QuotaBytes < 0 || (request.QuotaBytes > 0 && request.QuotaBytes < 1<<20) {
		return "", errors.New("hard quota must be zero or at least 1 MiB")
	}
	relative, err := validateNASRelativePath(request.RelativePath)
	if err != nil {
		return "", err
	}
	folder := filepath.Join(request.RootPath, ".home-ai", relative)
	if err := validateSMBSharePath(folder, uid, gid); err != nil {
		return "", err
	}
	limit := uint64(request.QuotaBytes / 1024)
	if limit > 0 && quota.CurrentSpace > limit*1024 {
		return "", errors.New("hard quota is below allocated filesystem space")
	}
	device, err := quotaDevice(ctx, request.RootPath)
	if err != nil {
		return "", err
	}
	// The tree is not writable through SMB while this operation runs. Refuse
	// cross-project hardlinks and symlinks rather than silently reassigning them.
	err = filepath.WalkDir(folder, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 || (!entry.IsDir() && !entry.Type().IsRegular()) {
			return errors.New("quota tree contains symlinks or special files")
		}
		fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			return err
		}
		defer unix.Close(fd)
		var stat unix.Stat_t
		if err := unix.Fstat(fd, &stat); err != nil {
			return err
		}
		if !entry.IsDir() && stat.Nlink > 1 {
			return errors.New("quota adoption refuses multiply linked files")
		}
		var attributes projectAttributes
		if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), fsGetProject, uintptr(unsafe.Pointer(&attributes))); errno != 0 {
			return errno
		}
		if attributes.ProjectID != 0 && attributes.ProjectID != request.ProjectID {
			return errors.New("folder already belongs to a different quota project")
		}
		attributes.ProjectID = request.ProjectID
		if entry.IsDir() {
			attributes.Flags |= projectInherit
		}
		if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), fsSetProject, uintptr(unsafe.Pointer(&attributes))); errno != 0 {
			return errno
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("prepare filesystem project: %w", err)
	}
	// Read accounting again after adoption: existing files must fit the cap.
	if err := projectQuotaCall(quotaGetProject, device, request.ProjectID, &quota); err != nil {
		return "", err
	}
	if limit > 0 && quota.CurrentSpace > limit*1024 {
		return "", errors.New("hard quota is below existing filesystem allocation")
	}
	quota.BlockHardLimit = limit
	quota.BlockSoftLimit = 0
	quota.Valid = quotaBlockLimits
	if err := projectQuotaCall(quotaSetProject, device, request.ProjectID, &quota); err != nil {
		return "", fmt.Errorf("set filesystem quota: %w", err)
	}
	var verified projectQuota
	if err := projectQuotaCall(quotaGetProject, device, request.ProjectID, &verified); err != nil {
		return "", err
	}
	if verified.BlockHardLimit != limit {
		return "", errors.New("filesystem did not retain the requested hard quota")
	}
	return "Filesystem hard quota applied", nil
}
