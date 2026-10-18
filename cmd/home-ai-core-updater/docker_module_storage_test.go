package main

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssignedNVRUUIDRequiresSingleValidDisk(t *testing.T) {
	previous := nvrStorageDBPath
	nvrStorageDBPath = filepath.Join(t.TempDir(), "core.db")
	t.Cleanup(func() { nvrStorageDBPath = previous })
	db, err := sql.Open("sqlite", nvrStorageDBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE storage_purposes (filesystem_uuid TEXT, purpose TEXT)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO storage_purposes VALUES (?, ?)", "6740f386-7f43-4f9a-be9f-617cf7899d67", "nvr"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO storage_purposes VALUES (?, ?)", "a0582793-20a8-473d-8a97-341f6aac4dcc", "files"); err != nil {
		t.Fatal(err)
	}
	uuid, err := assignedNVRUUID(context.Background())
	if err != nil || uuid != "6740f386-7f43-4f9a-be9f-617cf7899d67" {
		t.Fatalf("UUID=%q err=%v", uuid, err)
	}
	if _, err := db.Exec("INSERT INTO storage_purposes VALUES (?, ?)", "a0582793-20a8-473d-8a97-341f6aac4dcc", "nvr"); err != nil {
		t.Fatal(err)
	}
	if _, err := assignedNVRUUID(context.Background()); err == nil || !strings.Contains(err.Error(), "more than one") {
		t.Fatalf("multiple assigned NVR disks must fail closed: %v", err)
	}
}

func TestNVRCopyVerificationPreservesFilesAndDoesNotFollowLinks(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "original")
	target := filepath.Join(root, "on-disk")
	for _, p := range []string{source, target} {
		if err := os.Mkdir(p, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []string{source, target} {
		if err := os.WriteFile(filepath.Join(p, "nvr.db"), []byte("sample database"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("../../not-for-reading", filepath.Join(p, "symlink")); err != nil {
			t.Fatal(err)
		}
	}
	if err := verifyNVRCopy(source, target); err != nil {
		t.Fatalf("matching copy rejected: %v", err)
	}
	if _, err := safeNVRHash(filepath.Join(source, "symlink")); err == nil {
		t.Fatal("hash must never follow a source symlink")
	}
	if err := os.WriteFile(filepath.Join(target, "nvr.db"), []byte("tampered copy"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifyNVRCopy(source, target); err == nil {
		t.Fatal("modified copy must fail verification")
	}
}

func TestNVRMissingDiskNeverUsesSystemDirectory(t *testing.T) {
	previousDB, previousRoot := nvrStorageDBPath, nvrStorageRoot
	nvrStorageDBPath = filepath.Join(t.TempDir(), "core.db")
	nvrStorageRoot = t.TempDir()
	t.Cleanup(func() { nvrStorageDBPath, nvrStorageRoot = previousDB, previousRoot })
	db, err := sql.Open("sqlite", nvrStorageDBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE storage_purposes (filesystem_uuid TEXT, purpose TEXT)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO storage_purposes VALUES (?, ?)", "6740f386-7f43-4f9a-be9f-617cf7899d67", "nvr"); err != nil {
		t.Fatal(err)
	}
	// An ordinary empty directory at the expected mountpoint is NOT a disk.
	if err := os.Mkdir(filepath.Join(nvrStorageRoot, "sdc1"), 0o755); err != nil {
		t.Fatal(err)
	}
	moduleDir := filepath.Join(t.TempDir(), "nvr.module")
	if err := prepareFreshNVRStorage(context.Background(), moduleDir, os.Getuid(), os.Getgid()); err == nil {
		t.Fatal("NVR install must refuse an unmounted assigned disk")
	}
	if _, err := os.Lstat(moduleDir); !os.IsNotExist(err) {
		t.Fatalf("NVR fallback path should not be created on system disk: %v", err)
	}
}
