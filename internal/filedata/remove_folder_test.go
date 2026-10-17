package filedata

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveEmptyLogicalFolder(t *testing.T) {
	root := filepath.Join(t.TempDir(), "nsf_empty")
	if err := os.Mkdir(root, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := CheckEmptyLogicalFolder(root); err != nil {
		t.Fatalf("empty preflight: %v", err)
	}
	if err := RemoveEmptyLogicalFolder(root); err != nil {
		t.Fatalf("remove empty folder: %v", err)
	}
	if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("folder was not removed: %v", err)
	}
}

func TestRemoveEmptyLogicalFolderWithEmptyTrashScaffold(t *testing.T) {
	root := filepath.Join(t.TempDir(), "nsf_with_trash")
	for _, path := range []string{filepath.Join(root, trashDirName, "items"), filepath.Join(root, trashDirName, "meta")} {
		if err := os.MkdirAll(path, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := RemoveEmptyLogicalFolder(root); err != nil {
		t.Fatalf("empty trash scaffold should be removable: %v", err)
	}
	if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("folder was not removed: %v", err)
	}
}

func TestRemoveEmptyLogicalFolderRefusesDataAndTrash(t *testing.T) {
	for _, tc := range []struct {
		name, subPath string
		directory     bool
	}{
		{"file", "report.txt", false},
		{"subdirectory", "Reports", true},
		{"unfinished_upload", ".home-ai-upload-abc.part", false},
		{"trashed_item", filepath.Join(trashDirName, "items", "trash_test"), false},
		{"trash_metadata", filepath.Join(trashDirName, "meta", "trash_test.json"), false},
		{"unexpected_hidden", ".hidden", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "nsf_protected")
			if err := os.MkdirAll(filepath.Dir(filepath.Join(root, tc.subPath)), 0o750); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(root, tc.subPath)
			if tc.directory {
				if err := os.Mkdir(target, 0o750); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.WriteFile(target, []byte("protected user data"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := CheckEmptyLogicalFolder(root); !errors.Is(err, ErrLogicalFolderNotEmpty) {
				t.Fatalf("preflight must refuse content: %v", err)
			}
			if err := RemoveEmptyLogicalFolder(root); !errors.Is(err, ErrLogicalFolderNotEmpty) {
				t.Fatalf("removal must refuse content: %v", err)
			}
			if _, err := os.Lstat(target); err != nil {
				t.Fatalf("protected content was altered: %v", err)
			}
		})
	}
}

func TestRemoveEmptyLogicalFolderRejectsSymlinkScaffold(t *testing.T) {
	root := filepath.Join(t.TempDir(), "nsf_symlink")
	outside := t.TempDir()
	if err := os.Mkdir(root, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, trashDirName)); err != nil {
		t.Fatal(err)
	}
	if err := RemoveEmptyLogicalFolder(root); !errors.Is(err, ErrLogicalFolderNotEmpty) {
		t.Fatalf("symlink trash must be rejected: %v", err)
	}
	if _, err := os.Lstat(root); err != nil {
		t.Fatalf("root was changed: %v", err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside target was changed: %v", err)
	}
}
