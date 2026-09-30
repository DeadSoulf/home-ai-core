//go:build linux

package filedata

import (
	"errors"

	"golang.org/x/sys/unix"
)

// installUploadPart atomically commits an upload without replacing any existing
// destination entry, including a directory or a dangling symlink. The part and
// destination must be on the same filesystem.
func installUploadPart(partPath, target string) error {
	err := unix.Renameat2(unix.AT_FDCWD, partPath, unix.AT_FDCWD, target, unix.RENAME_NOREPLACE)
	if !errors.Is(err, unix.ENOSYS) {
		return err
	}
	// Old kernels without renameat2 can safely install using a hard link. A
	// filesystem that cannot provide either operation must fail without falling
	// back to a replacing rename.
	return installUploadPartByLink(partPath, target)
}
