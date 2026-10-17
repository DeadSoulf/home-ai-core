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

func TestInspectLogicalFolderForDeletionRevealsHiddenBlockers(t *testing.T) {
	for _, tc := range []struct {
		name                                     string
		paths                                    []string
		files                                    []string
		visible, uploads, items, metadata, other int
	}{
		{name: "empty"},
		{name: "empty trash scaffold", paths: []string{
			filepath.Join(trashDirName, "items"),
			filepath.Join(trashDirName, "meta"),
		}},
		{name: "ordinary file", files: []string{"visible.txt"}, visible: 1},
		{name: "orphan upload artifact hidden in file browser", files: []string{
			".home-ai-upload-orphan.part",
		}, uploads: 1},
		{name: "upload pair with invalid metadata", files: []string{
			".home-ai-upload-stale.part",
			".home-ai-upload-stale.json",
		}, uploads: 2},
		{name: "trashed file hidden in file browser", files: []string{
			filepath.Join(trashDirName, "items", "trash_id"),
			filepath.Join(trashDirName, "meta", "trash_id.json"),
		}, paths: []string{filepath.Join(trashDirName, "items"), filepath.Join(trashDirName, "meta")}, items: 1, metadata: 1},
		{name: "unknown orphan in trash", files: []string{
			filepath.Join(trashDirName, "orphan.tmp"),
		}, other: 1},
		{name: "incomplete trash scaffold", paths: []string{
			filepath.Join(trashDirName, "items"),
		}, other: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "logical-folder")
			if err := os.Mkdir(root, 0o750); err != nil {
				t.Fatal(err)
			}
			for _, name := range tc.paths {
				if err := os.MkdirAll(filepath.Join(root, name), 0o750); err != nil {
					t.Fatal(err)
				}
			}
			for _, name := range tc.files {
				target := filepath.Join(root, name)
				if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(target, []byte("retain me"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			got, err := InspectLogicalFolderForDeletion(root)
			if err != nil {
				t.Fatal(err)
			}
			if got.VisibleEntries != tc.visible || got.UploadArtifacts != tc.uploads || got.TrashItems != tc.items ||
				got.TrashMetadata != tc.metadata || got.OtherHidden != tc.other {
				t.Fatalf("blocker counts = %#v, want visible=%d uploads=%d items=%d metadata=%d other=%d",
					got, tc.visible, tc.uploads, tc.items, tc.metadata, tc.other)
			}
			if got.HasBlockers() {
				if err := RemoveEmptyLogicalFolder(root); !errors.Is(err, ErrLogicalFolderNotEmpty) {
					t.Fatalf("protected data must reject deletion: %v", err)
				}
				for _, name := range tc.files {
					if _, err := os.Lstat(filepath.Join(root, name)); err != nil {
						t.Fatalf("file lost: %s: %v", name, err)
					}
				}
			}
		})
	}
}

func TestInspectLogicalFolderForDeletionDoesNotFollowTrashSymlink(t *testing.T) {
	root := filepath.Join(t.TempDir(), "nsf")
	outside := t.TempDir()
	if err := os.Mkdir(root, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "user-data"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, trashDirName)); err != nil {
		t.Fatal(err)
	}
	got, err := InspectLogicalFolderForDeletion(root)
	if err != nil || got.OtherHidden != 1 {
		t.Fatalf("trash symlink should be an unknown blocker: %#v %v", got, err)
	}
	if err := RemoveEmptyLogicalFolder(root); !errors.Is(err, ErrLogicalFolderNotEmpty) {
		t.Fatalf("symlink deletion: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(outside, "user-data")); err != nil {
		t.Fatalf("symlink target lost: %v", err)
	}
}
