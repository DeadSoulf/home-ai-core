package filedata

import (
	"bytes"
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
