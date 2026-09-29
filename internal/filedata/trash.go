package filedata

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const trashDirName = ".home-ai-trash"

type TrashEntry struct {
	ID           string    `json:"id"`
	OriginalPath string    `json:"original_path"`
	Name         string    `json:"name"`
	Kind         string    `json:"kind"`
	SizeBytes    int64     `json:"size_bytes,omitempty"`
	DeletedAt    time.Time `json:"deleted_at"`
}

func Trash(root, relative string, now time.Time) (TrashEntry, error) {
	relative = cleanRelative(relative)
	if relative == "" {
		return TrashEntry{}, errors.New("logical folder root cannot be trashed")
	}
	source, err := resolveExisting(root, relative)
	if err != nil {
		return TrashEntry{}, err
	}
	info, err := os.Lstat(source)
	if err != nil {
		return TrashEntry{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return TrashEntry{}, errors.New("symlink trash is not supported")
	}

	itemsDir, metaDir, err := ensureTrashLayout(root)
	if err != nil {
		return TrashEntry{}, err
	}
	id, err := newTrashID()
	if err != nil {
		return TrashEntry{}, err
	}
	entry := TrashEntry{
		ID:           id,
		OriginalPath: filepath.ToSlash(relative),
		Name:         filepath.Base(relative),
		Kind:         "file",
		SizeBytes:    fileSize(info, "file"),
		DeletedAt:    now.UTC(),
	}
	if info.IsDir() {
		entry.Kind = "directory"
		entry.SizeBytes = 0
	}

	itemPath := filepath.Join(itemsDir, id)
	if err := os.Rename(source, itemPath); err != nil {
		return TrashEntry{}, fmt.Errorf("move entry to trash: %w", err)
	}
	if err := writeTrashMetadata(filepath.Join(metaDir, id+".json"), entry); err != nil {
		if rollbackErr := os.Rename(itemPath, source); rollbackErr != nil {
			return TrashEntry{}, fmt.Errorf("write trash metadata: %v; rollback failed: %v", err, rollbackErr)
		}
		return TrashEntry{}, err
	}
	return entry, nil
}

func ListTrash(root string) ([]TrashEntry, error) {
	_, metaDir, err := trashLayout(root, false)
	if errors.Is(err, os.ErrNotExist) {
		return []TrashEntry{}, nil
	}
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(metaDir)
	if err != nil {
		return nil, fmt.Errorf("read trash metadata: %w", err)
	}
	result := make([]TrashEntry, 0, len(entries))
	for _, item := range entries {
		if item.IsDir() || !strings.HasSuffix(item.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(item.Name(), ".json")
		if !validTrashID(id) {
			continue
		}
		entry, err := readTrashMetadata(filepath.Join(metaDir, item.Name()))
		if err != nil || entry.ID != id {
			continue
		}
		result = append(result, entry)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].DeletedAt.After(result[j].DeletedAt)
	})
	return result, nil
}

func RestoreTrash(root, id string) (TrashEntry, error) {
	itemsDir, metaDir, err := trashLayout(root, false)
	if err != nil {
		return TrashEntry{}, err
	}
	if !validTrashID(id) {
		return TrashEntry{}, errors.New("invalid trash identity")
	}
	metaPath := filepath.Join(metaDir, id+".json")
	entry, err := readTrashMetadata(metaPath)
	if err != nil {
		return TrashEntry{}, err
	}
	if entry.ID != id {
		return TrashEntry{}, errors.New("trash metadata identity mismatch")
	}

	parentRelative, name, err := splitTarget(entry.OriginalPath)
	if err != nil {
		return TrashEntry{}, err
	}
	parent, err := resolveExisting(root, parentRelative)
	if err != nil {
		return TrashEntry{}, fmt.Errorf("restore parent unavailable: %w", err)
	}
	target := filepath.Join(parent, name)
	if _, err := os.Lstat(target); err == nil {
		return TrashEntry{}, errors.New("restore target already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return TrashEntry{}, err
	}

	itemPath := filepath.Join(itemsDir, id)
	info, err := os.Lstat(itemPath)
	if err != nil {
		return TrashEntry{}, fmt.Errorf("inspect trash item: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return TrashEntry{}, errors.New("trash item is a symlink")
	}
	if err := os.Rename(itemPath, target); err != nil {
		return TrashEntry{}, fmt.Errorf("restore trash item: %w", err)
	}
	if err := os.Remove(metaPath); err != nil {
		return TrashEntry{}, fmt.Errorf("remove restored trash metadata: %w", err)
	}
	return entry, nil
}

func PurgeTrash(root, id string) (TrashEntry, error) {
	itemsDir, metaDir, err := trashLayout(root, false)
	if err != nil {
		return TrashEntry{}, err
	}
	if !validTrashID(id) {
		return TrashEntry{}, errors.New("invalid trash identity")
	}
	metaPath := filepath.Join(metaDir, id+".json")
	entry, err := readTrashMetadata(metaPath)
	if err != nil {
		return TrashEntry{}, err
	}
	itemPath := filepath.Join(itemsDir, id)
	info, err := os.Lstat(itemPath)
	if err != nil {
		return TrashEntry{}, fmt.Errorf("inspect trash item: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return TrashEntry{}, errors.New("trash item is a symlink")
	}
	if err := os.RemoveAll(itemPath); err != nil {
		return TrashEntry{}, fmt.Errorf("purge trash item: %w", err)
	}
	if err := os.Remove(metaPath); err != nil {
		return TrashEntry{}, fmt.Errorf("remove trash metadata: %w", err)
	}
	return entry, nil
}

func ensureTrashLayout(root string) (string, string, error) {
	itemsDir, metaDir, err := trashLayout(root, true)
	if err != nil {
		return "", "", err
	}
	return itemsDir, metaDir, nil
}

func trashLayout(root string, create bool) (string, string, error) {
	root = filepath.Clean(root)
	info, err := os.Lstat(root)
	if err != nil {
		return "", "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", "", errors.New("logical folder root must be a real directory")
	}

	trashRoot := filepath.Join(root, trashDirName)
	itemsDir := filepath.Join(trashRoot, "items")
	metaDir := filepath.Join(trashRoot, "meta")
	for _, path := range []string{trashRoot, itemsDir, metaDir} {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) && create {
			if err := os.Mkdir(path, 0o750); err != nil {
				return "", "", fmt.Errorf("create trash directory: %w", err)
			}
			continue
		}
		if err != nil {
			return "", "", err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return "", "", errors.New("trash path must be a real directory")
		}
	}
	return itemsDir, metaDir, nil
}

func writeTrashMetadata(path string, entry TrashEntry) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".home-ai-trash-meta-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o640); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(append(data, '\n')); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func readTrashMetadata(path string) (TrashEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return TrashEntry{}, fmt.Errorf("read trash metadata: %w", err)
	}
	var entry TrashEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return TrashEntry{}, fmt.Errorf("decode trash metadata: %w", err)
	}
	return entry, nil
}

func newTrashID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate trash identity: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

func validTrashID(value string) bool {
	if len(value) != 32 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
