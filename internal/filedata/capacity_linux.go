//go:build linux

package filedata

import (
	"fmt"
	"syscall"
)

type Capacity struct {
	TotalBytes uint64
	FreeBytes  uint64
}

func ReadCapacity(path string) (Capacity, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return Capacity{}, fmt.Errorf("read filesystem capacity: %w", err)
	}
	blockSize := uint64(stat.Bsize)
	return Capacity{
		TotalBytes: stat.Blocks * blockSize,
		FreeBytes:  stat.Bavail * blockSize,
	}, nil
}
