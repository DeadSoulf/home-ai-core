package files

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/state"
)

func TestFileOperationsStayInsideFolderRoot(t *testing.T) {
	pool := t.TempDir()
	folder := state.NASFolderRecord{
		PoolRoot:     pool,
		RelativePath: "shared/nsf_testfolder",
	}
	physical := FolderPath(folder)
	if err := os.MkdirAll(physical, 0o750); err != nil {
		t.Fatal(err)
	}

	if err := CreateDirectory(folder, "Photos/2026"); err != nil {
		t.Fatalf("CreateDirectory() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(physical, "Photos", "2026", "one.txt"), []byte("hello"), 0o640); err != nil {
		t.Fatal(err)
	}

	entries, err := List(folder, "Photos/2026")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "one.txt" || entries[0].Type != "file" {
		t.Fatalf("unexpected entries: %#v", entries)
	}

	if err := Move(folder, "Photos/2026/one.txt", "Photos/2026/two.txt"); err != nil {
		t.Fatalf("Move() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(physical, "Photos", "2026", "two.txt")); err != nil {
		t.Fatalf("moved file missing: %v", err)
	}

	file, info, closeFn, err := OpenDownloadWithClose(folder, "Photos/2026/two.txt")
	if err != nil {
		t.Fatalf("OpenDownloadWithClose() error = %v", err)
	}
	if info.Size() != 5 {
		t.Fatalf("download size = %d, want 5", info.Size())
	}
	closeFn()
	_ = file

	if err := Delete(folder, "Photos"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(physical, "Photos")); !os.IsNotExist(err) {
		t.Fatalf("deleted directory still exists: %v", err)
	}
}

func TestFileOperationsRejectTraversalAndRootMutation(t *testing.T) {
	pool := t.TempDir()
	folder := state.NASFolderRecord{
		PoolRoot:     pool,
		RelativePath: "shared/nsf_testfolder",
	}
	if err := os.MkdirAll(FolderPath(folder), 0o750); err != nil {
		t.Fatal(err)
	}

	for _, value := range []string{"../escape", "dir/../../escape", "/absolute"} {
		if _, err := List(folder, value); err == nil {
			t.Fatalf("List(%q) accepted unsafe path", value)
		}
	}
	if err := Delete(folder, ""); err != ErrRootMutation {
		t.Fatalf("Delete(root) error = %v, want ErrRootMutation", err)
	}
	if err := CreateDirectory(folder, ".."); err != ErrInvalidPath {
		t.Fatalf("CreateDirectory(..) error = %v, want ErrInvalidPath", err)
	}
}

func TestListOrdersDirectoriesBeforeFiles(t *testing.T) {
	pool := t.TempDir()
	folder := state.NASFolderRecord{
		PoolRoot:     pool,
		RelativePath: "shared/nsf_testfolder",
	}
	physical := FolderPath(folder)
	if err := os.MkdirAll(filepath.Join(physical, "Zulu"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(physical, "alpha.txt"), []byte("x"), 0o640); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := os.Chtimes(filepath.Join(physical, "alpha.txt"), now, now); err != nil {
		t.Fatal(err)
	}

	entries, err := List(folder, "")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 2 || entries[0].Type != "directory" || entries[1].Type != "file" {
		t.Fatalf("unexpected ordering: %#v", entries)
	}
}
