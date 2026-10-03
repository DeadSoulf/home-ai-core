//go:build linux

package nvr

import "syscall"

func (osSpaceChecker) Space(path string) (FilesystemSpace, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return FilesystemSpace{}, err
	}
	return FilesystemSpace{
		TotalBytes:     stat.Blocks * uint64(stat.Bsize),
		AvailableBytes: stat.Bavail * uint64(stat.Bsize),
	}, nil
}
