package frigate

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
)

// Snapshot excludes neither the SQLite WAL nor SHM files. The caller must first
// stop the only container that can write this configuration directory.
// Special files and symlinks are deliberately refused, rather than followed.
type SnapshotEntry struct {
	SHA256    string `json:"sha256,omitempty"`
	Size      int64  `json:"size"`
	Mode      uint32 `json:"mode"`
	Directory bool   `json:"directory,omitempty"`
}
type Snapshot map[string]SnapshotEntry

func ScanConfig(root string, maxBytes int64) (Snapshot, error) {
	out := Snapshot{}
	var total int64
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || (!info.Mode().IsRegular() && !info.IsDir()) {
			return fmt.Errorf("unsupported configuration file: %s", d.Name())
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			if !info.IsDir() {
				return errors.New("configuration root is not a directory")
			}
			return nil
		}
		key := filepath.ToSlash(rel)
		entry := SnapshotEntry{Size: info.Size(), Mode: uint32(info.Mode().Perm()), Directory: info.IsDir()}
		if info.IsDir() {
			entry.Size = 0
			out[key] = entry
			return nil
		}
		total += info.Size()
		if total > maxBytes {
			return errors.New("configuration snapshot exceeds configured size limit")
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		h := sha256.New()
		n, copyErr := io.Copy(h, io.LimitReader(file, maxBytes+1))
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if n != info.Size() {
			return errors.New("configuration changed while snapshotting")
		}
		entry.SHA256 = hex.EncodeToString(h.Sum(nil))
		out[key] = entry
		return nil
	})
	return out, err
}

func CopyConfig(src, dst string, maxBytes int64) (Snapshot, error) {
	expected, err := ScanConfig(src, maxBytes)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(src, dst)
	if err != nil {
		return nil, err
	}
	if rel == "." || (rel != ".." && !startsWithParent(rel)) {
		return nil, errors.New("snapshot destination must not be inside source")
	}
	if err := os.Mkdir(dst, 0700); err != nil {
		return nil, err
	}
	completed := false
	defer func() {
		if !completed {
			_ = os.RemoveAll(dst)
		}
	}()
	err = filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		entry, ok := expected[filepath.ToSlash(rel)]
		if !ok {
			return errors.New("configuration tree changed during snapshot")
		}
		target := filepath.Join(dst, rel)
		if entry.Directory {
			return os.Mkdir(target, 0700)
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		defer input.Close()
		output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, os.FileMode(entry.Mode))
		if err != nil {
			return err
		}
		h := sha256.New()
		n, copyErr := io.Copy(io.MultiWriter(output, h), io.LimitReader(input, entry.Size+1))
		modeErr := output.Chmod(os.FileMode(entry.Mode))
		syncErr := output.Sync()
		closeErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		if modeErr != nil {
			return modeErr
		}
		if syncErr != nil {
			return syncErr
		}
		if closeErr != nil {
			return closeErr
		}
		if n != entry.Size || hex.EncodeToString(h.Sum(nil)) != entry.SHA256 {
			return errors.New("configuration changed during snapshot")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// The root remains private. Restore exact entry modes despite helper UMask=0077.
	for name, entry := range expected {
		if entry.Directory {
			if err := os.Chmod(filepath.Join(dst, filepath.FromSlash(name)), os.FileMode(entry.Mode)); err != nil {
				return nil, err
			}
		}
	}
	if err := VerifyConfig(dst, expected, maxBytes); err != nil {
		return nil, err
	}
	// Flush directory entries as well as file contents before exposing a backup.
	err = filepath.WalkDir(dst, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return SyncDirectory(path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	completed = true
	return expected, nil
}
func startsWithParent(rel string) bool {
	return len(rel) > 3 && rel[:3] == ".."+string(filepath.Separator)
}
func VerifyConfig(root string, expected Snapshot, maxBytes int64) error {
	actual, err := ScanConfig(root, maxBytes)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(actual, expected) {
		return errors.New("configuration backup integrity verification failed")
	}
	return nil
}
func SyncDirectory(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
