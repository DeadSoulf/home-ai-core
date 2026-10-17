package filedata

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// ErrLogicalFolderNotEmpty prevents deleting a folder while it contains user
// data, unfinished uploads, recoverable trash, or unknown hidden entries.
var ErrLogicalFolderNotEmpty = errors.New("logical folder contains files, directories, uploads or trashed items")

// CheckEmptyLogicalFolder never writes. It allows only a completely empty
// logical folder or an empty internal trash-directory scaffold.
func CheckEmptyLogicalFolder(root string) error {
	info, err := os.Lstat(root)
	if err != nil {
		return fmt.Errorf("inspect logical folder before deletion: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("refusing to delete a symlink or non-directory logical folder")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("read logical folder before deletion: %w", err)
	}
	if len(entries) == 0 {
		return nil
	}
	if len(entries) != 1 || entries[0].Name() != trashDirName {
		return ErrLogicalFolderNotEmpty
	}
	trashRoot := filepath.Join(root, trashDirName)
	info, err = os.Lstat(trashRoot)
	if err != nil {
		return fmt.Errorf("inspect logical folder trash: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return ErrLogicalFolderNotEmpty
	}
	children, err := os.ReadDir(trashRoot)
	if err != nil {
		return fmt.Errorf("inspect logical folder trash: %w", err)
	}
	if len(children) != 2 {
		return ErrLogicalFolderNotEmpty
	}
	for _, name := range []string{"items", "meta"} {
		sub := filepath.Join(trashRoot, name)
		info, err := os.Lstat(sub)
		if err != nil {
			return fmt.Errorf("inspect logical folder trash: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return ErrLogicalFolderNotEmpty
		}
		entries, err := os.ReadDir(sub)
		if err != nil {
			return fmt.Errorf("inspect logical folder trash: %w", err)
		}
		if len(entries) > 0 {
			return ErrLogicalFolderNotEmpty
		}
	}
	for _, child := range children {
		if child.Name() != "items" && child.Name() != "meta" {
			return ErrLogicalFolderNotEmpty
		}
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
