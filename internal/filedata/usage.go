package filedata

import (
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
)

type Usage struct {
	UsedBytes     int64 `json:"used_bytes"`
	ReservedBytes int64 `json:"reserved_bytes"`
}

// UsageOf includes the recycle bin, and reserves the complete size of pending
// uploads. Sparse files are charged by apparent size, so writing holes cannot
// evade a logical quota. Symlinks and special files are never followed.
func UsageOf(root string) (Usage, error) {
	var result Usage
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("NAS usage contains a symlink")
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return errors.New("NAS usage contains a special file")
		}
		name := entry.Name()
		if filepath.Dir(path) == root && (strings.HasPrefix(name, ".home-ai-upload-") || strings.HasPrefix(name, ".home-ai-upload.")) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Size() > math.MaxInt64-result.UsedBytes {
			return errors.New("NAS usage overflows")
		}
		result.UsedBytes += info.Size()
		return nil
	})
	if err != nil {
		return Usage{}, fmt.Errorf("read folder usage: %w", err)
	}
	uploads, err := ListUploads(root)
	if err != nil {
		return Usage{}, err
	}
	for _, upload := range uploads {
		if upload.TotalBytes < 0 || upload.TotalBytes > math.MaxInt64-result.ReservedBytes {
			return Usage{}, errors.New("invalid upload reservation")
		}
		result.ReservedBytes += upload.TotalBytes
	}
	return result, nil
}
