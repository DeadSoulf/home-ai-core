package files

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/state"
)

var (
	ErrInvalidPath = errors.New("invalid file path")
	ErrNotDirectory = errors.New("path is not a directory")
	ErrNotRegular = errors.New("path is not a regular file")
	ErrDestinationExists = errors.New("destination already exists")
	ErrRootMutation = errors.New("folder root cannot be modified")
)

type Entry struct {
	Name       string    `json:"name"`
	Path       string    `json:"path"`
	Type       string    `json:"type"`
	SizeBytes  int64     `json:"size_bytes,omitempty"`
	ModifiedAt time.Time `json:"modified_at"`
}

func FolderPath(record state.NASFolderRecord) string {
	return filepath.Join(
		record.PoolRoot,
		".home-ai",
		filepath.FromSlash(record.RelativePath),
	)
}

func List(record state.NASFolderRecord, requested string) ([]Entry, error) {
	relative, err := cleanRelativePath(requested, true)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(FolderPath(record))
	if err != nil {
		return nil, fmt.Errorf("open managed folder: %w", err)
	}
	defer root.Close()

	dir, err := root.Open(relative)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	info, err := dir.Stat()
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, ErrNotDirectory
	}

	items, err := dir.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	result := make([]Entry, 0, len(items))
	for _, item := range items {
		info, err := item.Info()
		if err != nil {
			return nil, err
		}
		itemPath := item.Name()
		if relative != "." {
			itemPath = pathpkg.Join(relative, item.Name())
		}
		result = append(result, Entry{
			Name:       item.Name(),
			Path:       itemPath,
			Type:       entryType(item, info),
			SizeBytes:  regularFileSize(info),
			ModifiedAt: info.ModTime().UTC(),
		})
	}
	sort.Slice(result, func(i, j int) bool {
		leftDir := result[i].Type == "directory"
		rightDir := result[j].Type == "directory"
		if leftDir != rightDir {
			return leftDir
		}
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})
	return result, nil
}

// OpenDownloadWithClose returns a regular file plus a close function that also
// closes the root descriptor. It is the preferred download API.
func OpenDownloadWithClose(record state.NASFolderRecord, requested string) (*os.File, fs.FileInfo, func(), error) {
	relative, err := cleanRelativePath(requested, false)
	if err != nil {
		return nil, nil, nil, err
	}
	root, err := os.OpenRoot(FolderPath(record))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("open managed folder: %w", err)
	}
	file, err := root.Open(relative)
	if err != nil {
		root.Close()
		return nil, nil, nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		root.Close()
		return nil, nil, nil, err
	}
	if !info.Mode().IsRegular() {
		file.Close()
		root.Close()
		return nil, nil, nil, ErrNotRegular
	}
	closeFn := func() {
		_ = file.Close()
		_ = root.Close()
	}
	return file, info, closeFn, nil
}

func CreateDirectory(record state.NASFolderRecord, requested string) error {
	relative, err := cleanRelativePath(requested, false)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(FolderPath(record))
	if err != nil {
		return fmt.Errorf("open managed folder: %w", err)
	}
	defer root.Close()
	if _, err := root.Lstat(relative); err == nil {
		return ErrDestinationExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := root.MkdirAll(relative, 0o750); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	return nil
}

func Delete(record state.NASFolderRecord, requested string) error {
	relative, err := cleanRelativePath(requested, false)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(FolderPath(record))
	if err != nil {
		return fmt.Errorf("open managed folder: %w", err)
	}
	defer root.Close()
	if err := root.RemoveAll(relative); err != nil {
		return fmt.Errorf("delete path: %w", err)
	}
	return nil
}

func Move(record state.NASFolderRecord, source, destination string) error {
	from, err := cleanRelativePath(source, false)
	if err != nil {
		return err
	}
	to, err := cleanRelativePath(destination, false)
	if err != nil {
		return err
	}
	if from == to {
		return nil
	}
	root, err := os.OpenRoot(FolderPath(record))
	if err != nil {
		return fmt.Errorf("open managed folder: %w", err)
	}
	defer root.Close()

	if _, err := root.Lstat(to); err == nil {
		return ErrDestinationExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := root.Rename(from, to); err != nil {
		return fmt.Errorf("move path: %w", err)
	}
	return nil
}

func cleanRelativePath(value string, allowRoot bool) (string, error) {
	if strings.IndexByte(value, 0) >= 0 || len(value) > 4096 {
		return "", ErrInvalidPath
	}
	value = strings.ReplaceAll(value, "\\", "/")
	if strings.HasPrefix(value, "/") {
		return "", ErrInvalidPath
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == ".." {
			return "", ErrInvalidPath
		}
	}
	clean := pathpkg.Clean(value)
	if clean == "." {
		if allowRoot {
			return ".", nil
		}
		return "", ErrRootMutation
	}
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", ErrInvalidPath
	}
	return clean, nil
}

func entryType(entry fs.DirEntry, info fs.FileInfo) string {
	if entry.Type()&os.ModeSymlink != 0 {
		return "symlink"
	}
	if info.IsDir() {
		return "directory"
	}
	if info.Mode().IsRegular() {
		return "file"
	}
	return "other"
}

func regularFileSize(info fs.FileInfo) int64 {
	if info.Mode().IsRegular() {
		return info.Size()
	}
	return 0
}
