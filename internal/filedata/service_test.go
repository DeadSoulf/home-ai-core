package filedata

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestListCreateUploadDownload(t *testing.T) {
	root := filepath.Join(t.TempDir(), "folder")
	if err := os.MkdirAll(root, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := CreateDirectory(root, "docs"); err != nil {
		t.Fatalf("CreateDirectory() error = %v", err)
	}
	if _, err := Upload(root, "docs/hello.txt", bytes.NewBufferString("hello")); err != nil {
		t.Fatalf("Upload() error = %v", err)
	}

	entries, err := List(root, "")
	if err != nil {
		t.Fatalf("List(root) error = %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "docs" || entries[0].Kind != "directory" {
		t.Fatalf("root entries = %#v", entries)
	}

	entries, err = List(root, "docs")
	if err != nil {
		t.Fatalf("List(docs) error = %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "hello.txt" || entries[0].Kind != "file" || entries[0].SizeBytes != 5 {
		t.Fatalf("docs entries = %#v", entries)
	}

	file, info, err := OpenFile(root, "docs/hello.txt")
	if err != nil {
		t.Fatalf("OpenFile() error = %v", err)
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" || info.Size() != 5 {
		t.Fatalf("download = %q size=%d", data, info.Size())
	}
}

func TestUploadLimitedRejectsBeforeCommit(t *testing.T) {
	root := t.TempDir()
	if _, err := UploadLimited(root, "too-large.txt", bytes.NewBufferString("hello"), 4); !errors.Is(err, ErrCapacityLimit) {
		t.Fatalf("capacity error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "too-large.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("limited upload committed unexpectedly: %v", err)
	}
	if _, err := UploadLimited(root, "exact.txt", bytes.NewBufferString("hello"), 5); err != nil {
		t.Fatalf("exact-limit upload error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "exact.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Fatalf("exact-limit upload = %q", data)
	}
}

func TestTraversalAndSymlinkAreRejected(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "folder")
	outside := filepath.Join(base, "outside")
	if err := os.MkdirAll(root, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{"../outside", "escape", "escape/file.txt"} {
		if _, err := List(root, path); err == nil {
			t.Fatalf("List(%q) unexpectedly succeeded", path)
		}
	}
	if _, err := Upload(root, "../outside.txt", bytes.NewBufferString("bad")); err == nil {
		t.Fatal("upload traversal unexpectedly succeeded")
	}
	if err := CreateDirectory(root, "escape/new"); err == nil {
		t.Fatal("directory creation through symlink unexpectedly succeeded")
	}
}

func TestFolderRoot(t *testing.T) {
	pool := t.TempDir()
	want := filepath.Join(pool, ".home-ai", "users", "usr_test", "nsf_test")
	if err := os.MkdirAll(want, 0o750); err != nil {
		t.Fatal(err)
	}
	got, err := FolderRoot(pool, "users/usr_test/nsf_test")
	if err != nil {
		t.Fatalf("FolderRoot() error = %v", err)
	}
	if got != want {
		t.Fatalf("FolderRoot() = %q, want %q", got, want)
	}
}

func TestDeleteAndMove(t *testing.T) {
	root := filepath.Join(t.TempDir(), "folder")
	if err := os.MkdirAll(filepath.Join(root, "docs", "sub"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "a.txt"), []byte("a"), 0o640); err != nil {
		t.Fatal(err)
	}

	if err := Move(root, "docs/a.txt", "docs/b.txt"); err != nil {
		t.Fatalf("Move(file) error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "docs", "b.txt")); err != nil {
		t.Fatalf("moved file missing: %v", err)
	}

	if err := Move(root, "docs/sub", "renamed"); err != nil {
		t.Fatalf("Move(directory) error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "renamed")); err != nil {
		t.Fatalf("moved directory missing: %v", err)
	}

	if err := Delete(root, "docs/b.txt"); err != nil {
		t.Fatalf("Delete(file) error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "docs", "b.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleted file still exists: %v", err)
	}
	if err := Delete(root, "renamed"); err != nil {
		t.Fatalf("Delete(empty directory) error = %v", err)
	}
}

func TestDeleteAndMoveSafety(t *testing.T) {
	root := filepath.Join(t.TempDir(), "folder")
	if err := os.MkdirAll(filepath.Join(root, "docs", "child"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "exists.txt"), []byte("x"), 0o640); err != nil {
		t.Fatal(err)
	}

	if err := Delete(root, ""); err == nil {
		t.Fatal("logical root deletion succeeded")
	}
	if err := Delete(root, "docs"); err == nil {
		t.Fatal("non-empty directory deletion succeeded")
	}
	if err := Move(root, "docs", "docs/child/moved"); err == nil {
		t.Fatal("directory moved inside itself")
	}
	if err := Move(root, "docs", "exists.txt"); err == nil {
		t.Fatal("move overwrote existing target")
	}
	if err := Move(root, "../outside", "x"); err == nil {
		t.Fatal("move traversal succeeded")
	}
}
