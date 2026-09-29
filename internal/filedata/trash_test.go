package filedata

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTrashRestoreAndPurge(t *testing.T) {
	root := filepath.Join(t.TempDir(), "folder")
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "a.txt"), []byte("hello"), 0o640); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	entry, err := Trash(root, "docs/a.txt", now)
	if err != nil {
		t.Fatalf("Trash() error = %v", err)
	}
	if entry.OriginalPath != "docs/a.txt" || entry.Kind != "file" || entry.SizeBytes != 5 {
		t.Fatalf("unexpected trash entry: %#v", entry)
	}
	if _, err := os.Stat(filepath.Join(root, "docs", "a.txt")); !os.IsNotExist(err) {
		t.Fatalf("source still exists after trash: %v", err)
	}

	items, err := ListTrash(root)
	if err != nil {
		t.Fatalf("ListTrash() error = %v", err)
	}
	if len(items) != 1 || items[0].ID != entry.ID {
		t.Fatalf("trash items = %#v", items)
	}

	restored, err := RestoreTrash(root, entry.ID)
	if err != nil {
		t.Fatalf("RestoreTrash() error = %v", err)
	}
	if restored.OriginalPath != "docs/a.txt" {
		t.Fatalf("restored entry = %#v", restored)
	}
	if data, err := os.ReadFile(filepath.Join(root, "docs", "a.txt")); err != nil || string(data) != "hello" {
		t.Fatalf("restored data = %q err=%v", data, err)
	}

	entry, err = Trash(root, "docs/a.txt", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("second Trash() error = %v", err)
	}
	if _, err := PurgeTrash(root, entry.ID); err != nil {
		t.Fatalf("PurgeTrash() error = %v", err)
	}
	items, err = ListTrash(root)
	if err != nil {
		t.Fatalf("ListTrash() after purge error = %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("trash not empty after purge: %#v", items)
	}
}

func TestTrashRestoreRefusesOverwrite(t *testing.T) {
	root := filepath.Join(t.TempDir(), "folder")
	if err := os.MkdirAll(root, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("old"), 0o640); err != nil {
		t.Fatal(err)
	}
	entry, err := Trash(root, "a.txt", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("new"), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := RestoreTrash(root, entry.ID); err == nil {
		t.Fatal("restore overwrote existing target")
	}
	data, err := os.ReadFile(filepath.Join(root, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Fatalf("existing target changed: %q", data)
	}
}

func TestTrashDirectoryAndReservedPathHidden(t *testing.T) {
	root := filepath.Join(t.TempDir(), "folder")
	if err := os.MkdirAll(filepath.Join(root, "docs", "child"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "child", "x.txt"), []byte("x"), 0o640); err != nil {
		t.Fatal(err)
	}
	entry, err := Trash(root, "docs", time.Now())
	if err != nil {
		t.Fatalf("Trash(directory) error = %v", err)
	}
	if entry.Kind != "directory" {
		t.Fatalf("kind = %q, want directory", entry.Kind)
	}

	entries, err := List(root, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range entries {
		if item.Name == trashDirName {
			t.Fatalf("internal trash directory leaked into listing: %#v", entries)
		}
	}
	if _, err := List(root, trashDirName); err == nil {
		t.Fatal("direct trash path browsing unexpectedly succeeded")
	}
}
