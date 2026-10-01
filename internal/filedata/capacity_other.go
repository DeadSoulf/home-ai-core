//go:build !linux

package filedata

import "errors"

type Capacity struct {
	TotalBytes uint64
	FreeBytes  uint64
}

func ReadCapacity(string) (Capacity, error) {
	return Capacity{}, errors.New("filesystem capacity inspection is unsupported on this platform")
}
