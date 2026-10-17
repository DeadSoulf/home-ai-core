package filedata

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// ErrLogicalFolderNotEmpty prevents deleting a folder while it contains user
// data, unfinished uploads, recoverable trash, or unknown hidden entries.
var ErrLogicalFolderNotEmpty = errors.New("logical folder contains files, directories, uploads or trashed items")

// FolderRemovalBlockers reports which entries hidden from ordinary file listing
// prevent deletion. Counts are diagnostic only: nothing here removes data.
type FolderRemovalBlockers struct {
	VisibleEntries  int `json:"visible_entries"`
	UploadArtifacts int `json:"upload_artifacts"`
	TrashItems      int `json:"trash_items"`
	TrashMetadata   int `json:"trash_metadata"`
	OtherHidden     int `json:"other_hidden"`
}

func (b FolderRemovalBlockers) HasBlockers() bool {
	return b.VisibleEntries > 0 || b.UploadArtifacts > 0 ||
		b.TrashItems > 0 || b.TrashMetadata > 0 || b.OtherHidden > 0
}

// InspectLogicalFolderForDeletion uses Lstat/ReadDir and never follows
// symlinks or removes files. A malformed trash layout fails closed.
// Upload artifacts are counted whether or not their session metadata is valid.
func InspectLogicalFolderForDeletion(root string) (FolderRemovalBlockers, error) {
	var blockers FolderRemovalBlockers
	info, err := os.Lstat(root)
	if err != nil {
		return blockers, fmt.Errorf("inspect logical folder before deletion: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return blockers, errors.New("refusing to delete a symlink or non-directory logical folder")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return blockers, fmt.Errorf("read logical folder before deletion: %w", err)
	}
	for _, entry := range entries {
		switch {
		case entry.Name() == trashDirName:
			if err := inspectFolderTrashForDeletion(root, &blockers); err != nil {
				return blockers, err
			}
		case strings.HasPrefix(entry.Name(), uploadPrefix):
			blockers.UploadArtifacts++
		default:
			blockers.VisibleEntries++
		}
	}
	return blockers, nil
}

func inspectFolderTrashForDeletion(root string, blockers *FolderRemovalBlockers) error {
	trashRoot := filepath.Join(root, trashDirName)
	info, err := os.Lstat(trashRoot)
	if err != nil {
		return fmt.Errorf("inspect logical folder trash: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		blockers.OtherHidden++
		return nil
	}
	children, err := os.ReadDir(trashRoot)
	if err != nil {
		return fmt.Errorf("inspect logical folder trash: %w", err)
	}
	found := map[string]bool{}
	for _, child := range children {
		name := child.Name()
		if name != "items" && name != "meta" {
			blockers.OtherHidden++
			continue
		}
		found[name] = true
		dir := filepath.Join(trashRoot, name)
		info, err := os.Lstat(dir)
		if err != nil {
			return fmt.Errorf("inspect logical folder trash: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			blockers.OtherHidden++
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return fmt.Errorf("inspect logical folder trash: %w", err)
		}
		if name == "items" {
			blockers.TrashItems += len(entries)
		} else {
			blockers.TrashMetadata += len(entries)
		}
	}
	if !found["items"] || !found["meta"] {
		blockers.OtherHidden++
	}
	return nil
}

// CheckEmptyLogicalFolder never writes. It allows only a completely empty
// logical folder or an empty internal trash-directory scaffold.
func CheckEmptyLogicalFolder(root string) error {
	blockers, err := InspectLogicalFolderForDeletion(root)
	if err != nil {
		return err
	}
	if blockers.HasBlockers() {
		return ErrLogicalFolderNotEmpty
	}
	return nil
}

// RemoveEmptyLogicalFolder uses non-recursive rmdir only. No ordinary file,
// subdirectory, upload or trash item can be unlinked by this operation.
// The caller verifies folder identity and permission and suspends SMB writes.
func RemoveEmptyLogicalFolder(root string) error {
	if err := CheckEmptyLogicalFolder(root); err != nil {
		return err
	}
	trashRoot := filepath.Join(root, trashDirName)
	if info, err := os.Lstat(trashRoot); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return ErrLogicalFolderNotEmpty
		}
		for _, name := range []string{"items", "meta"} {
			if err := os.Remove(filepath.Join(trashRoot, name)); err != nil {
				return translateDeleteFolderError("remove empty trash directory", err)
			}
		}
		if err := os.Remove(trashRoot); err != nil {
			return translateDeleteFolderError("remove empty trash scaffold", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect trash scaffold: %w", err)
	}
	if err := os.Remove(root); err != nil {
		return translateDeleteFolderError("remove empty logical folder", err)
	}
	return nil
}

func translateDeleteFolderError(operation string, err error) error {
	if errors.Is(err, syscall.ENOTEMPTY) || errors.Is(err, syscall.EEXIST) {
		return ErrLogicalFolderNotEmpty
	}
	return fmt.Errorf("%s: %w", operation, err)
}
