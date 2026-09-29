package filedata

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const dataRootName = ".home-ai"

type Entry struct {
	Name       string    `json:"name"`
	Path       string    `json:"path"`
	Kind       string    `json:"kind"`
	SizeBytes  int64     `json:"size_bytes,omitempty"`
	ModifiedAt time.Time `json:"modified_at"`
}

func FolderRoot(poolRoot, relativePath string) (string, error) {
	poolRoot = filepath.Clean(strings.TrimSpace(poolRoot))
	relativePath = filepath.Clean(strings.TrimSpace(relativePath))
	if poolRoot == "." || !filepath.IsAbs(poolRoot) {
		return "", errors.New("invalid pool root")
	}
	if relativePath == "." || filepath.IsAbs(relativePath) || pathEscapes(relativePath) {
		return "", errors.New("invalid folder relative path")
	}
	root := filepath.Join(poolRoot, dataRootName, relativePath)
	info, err := os.Lstat(root)
	if err != nil {
		return "", fmt.Errorf("inspect logical folder root: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", errors.New("logical folder root must be a real directory")
	}
	return root, nil
}

func List(root, relative string) ([]Entry, error) {
	dir, err := resolveExisting(root, relative)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("requested path is not a directory")
	}

	items, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read directory: %w", err)
	}
	result := make([]Entry, 0, len(items))
	for _, item := range items {
		if strings.HasPrefix(item.Name(), ".home-ai-upload-") {
			continue
		}
		itemPath := filepath.Join(dir, item.Name())
		info, err := os.Lstat(itemPath)
		if err != nil {
			return nil, fmt.Errorf("inspect entry %s: %w", item.Name(), err)
		}
		kind := "file"
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			kind = "symlink"
		case info.IsDir():
			kind = "directory"
		}
		entryRelative := filepath.ToSlash(filepath.Join(cleanRelative(relative), item.Name()))
		if entryRelative == "." {
			entryRelative = item.Name()
		}
		result = append(result, Entry{
			Name:       item.Name(),
			Path:       entryRelative,
			Kind:       kind,
			SizeBytes:  fileSize(info, kind),
			ModifiedAt: info.ModTime().UTC(),
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Kind != result[j].Kind {
			if result[i].Kind == "directory" {
				return true
			}
			if result[j].Kind == "directory" {
				return false
			}
		}
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})
	return result, nil
}

func CreateDirectory(root, relative string) error {
	parentRelative, name, err := splitTarget(relative)
	if err != nil {
		return err
	}
	parent, err := resolveExisting(root, parentRelative)
	if err != nil {
		return err
	}
	parentInfo, err := os.Lstat(parent)
	if err != nil {
		return err
	}
	if !parentInfo.IsDir() {
		return errors.New("parent path is not a directory")
	}
	target := filepath.Join(parent, name)
	if _, err := os.Lstat(target); err == nil {
		return errors.New("target already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Mkdir(target, 0o750); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	return nil
}

func Upload(root, relative string, source io.Reader) (int64, error) {
	parentRelative, name, err := splitTarget(relative)
	if err != nil {
		return 0, err
	}
	parent, err := resolveExisting(root, parentRelative)
	if err != nil {
		return 0, err
	}
	parentInfo, err := os.Lstat(parent)
	if err != nil {
		return 0, err
	}
	if !parentInfo.IsDir() {
		return 0, errors.New("parent path is not a directory")
	}

	target := filepath.Join(parent, name)
	if info, err := os.Lstat(target); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return 0, errors.New("target exists and is not a regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return 0, err
	}

	temp, err := os.CreateTemp(parent, ".home-ai-upload-*")
	if err != nil {
		return 0, fmt.Errorf("create upload temp file: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)

	if err := temp.Chmod(0o640); err != nil {
		_ = temp.Close()
		return 0, err
	}
	written, copyErr := io.Copy(temp, source)
	if copyErr != nil {
		_ = temp.Close()
		return written, fmt.Errorf("write upload: %w", copyErr)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return written, err
	}
	if err := temp.Close(); err != nil {
		return written, err
	}
	if err := os.Rename(tempPath, target); err != nil {
		return written, fmt.Errorf("commit upload: %w", err)
	}
	return written, nil
}

func Delete(root, relative string) error {
	relative = cleanRelative(relative)
	if relative == "" {
		return errors.New("logical folder root cannot be deleted")
	}
	path, err := resolveExisting(root, relative)
	if err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("symlink deletion is not supported")
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("delete entry: %w", err)
	}
	return nil
}

func Move(root, from, to string) error {
	from = cleanRelative(from)
	to = cleanRelative(to)
	if from == "" || to == "" {
		return errors.New("source and target paths are required")
	}
	if from == to {
		return errors.New("source and target paths are the same")
	}

	source, err := resolveExisting(root, from)
	if err != nil {
		return err
	}
	sourceInfo, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if sourceInfo.Mode()&os.ModeSymlink != 0 {
		return errors.New("symlink move is not supported")
	}

	targetParentRelative, targetName, err := splitTarget(to)
	if err != nil {
		return err
	}
	targetParent, err := resolveExisting(root, targetParentRelative)
	if err != nil {
		return err
	}
	targetParentInfo, err := os.Lstat(targetParent)
	if err != nil {
		return err
	}
	if !targetParentInfo.IsDir() {
		return errors.New("target parent is not a directory")
	}

	if sourceInfo.IsDir() &&
		(targetParent == source || strings.HasPrefix(targetParent, source+string(filepath.Separator))) {
		return errors.New("directory cannot be moved inside itself")
	}

	target := filepath.Join(targetParent, targetName)
	if _, err := os.Lstat(target); err == nil {
		return errors.New("target already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if err := os.Rename(source, target); err != nil {
		return fmt.Errorf("move entry: %w", err)
	}
	return nil
}

func OpenFile(root, relative string) (*os.File, os.FileInfo, error) {
	path, err := resolveExisting(root, relative)
	if err != nil {
		return nil, nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, nil, errors.New("requested path is not a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	return file, info, nil
}

func resolveExisting(root, relative string) (string, error) {
	root = filepath.Clean(root)
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return "", fmt.Errorf("inspect logical folder root: %w", err)
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return "", errors.New("logical folder root must be a real directory")
	}

	relative = cleanRelative(relative)
	if relative == "" {
		return root, nil
	}
	if filepath.IsAbs(relative) || pathEscapes(relative) {
		return "", errors.New("path escapes logical folder")
	}

	current := root
	for _, segment := range strings.Split(relative, string(filepath.Separator)) {
		if segment == "" || segment == "." || segment == ".." {
			return "", errors.New("invalid path segment")
		}
		current = filepath.Join(current, segment)
		info, err := os.Lstat(current)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return "", os.ErrNotExist
			}
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("symlink traversal is not allowed")
		}
	}
	return current, nil
}

func splitTarget(relative string) (string, string, error) {
	relative = cleanRelative(relative)
	if relative == "" || filepath.IsAbs(relative) || pathEscapes(relative) {
		return "", "", errors.New("invalid target path")
	}
	name := filepath.Base(relative)
	if name == "." || name == ".." || name == "" {
		return "", "", errors.New("invalid target name")
	}
	parent := filepath.Dir(relative)
	if parent == "." {
		parent = ""
	}
	return parent, name, nil
}

func cleanRelative(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", string(filepath.Separator)))
	if value == "" || value == "." {
		return ""
	}
	return filepath.Clean(value)
}

func pathEscapes(relative string) bool {
	return relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func fileSize(info os.FileInfo, kind string) int64 {
	if kind != "file" {
		return 0
	}
	return info.Size()
}
