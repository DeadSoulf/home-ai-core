package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNVRVideoArchiveMigrationPreservesOriginalsAndRejectsConflicts(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "old", "recordings")
	destination := filepath.Join(base, "new", "recordings")
	camera := filepath.Join(source, "cam1")
	if err := os.MkdirAll(camera, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(destination, 0o700); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(camera, "20261017_080000.mp4")
	if err := os.WriteFile(filename, []byte("original-archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	required, err := estimateNVRStorageTransfer(context.Background(), source, destination)
	if err != nil || required != uint64(len("original-archive")) {
		t.Fatalf("capacity estimate %d %v", required, err)
	}
	if err := copyNVRRecordings(context.Background(), source, destination, os.Getuid(), os.Getgid()); err != nil {
		t.Fatal(err)
	}
	copyPath := filepath.Join(destination, "cam1", filepath.Base(filename))
	copied, err := os.ReadFile(copyPath)
	if err != nil || string(copied) != "original-archive" {
		t.Fatalf("archive copy was corrupted: %q %v", copied, err)
	}
	original, err := os.ReadFile(filename)
	if err != nil || string(original) != "original-archive" {
		t.Fatalf("original recording not preserved: %q %v", original, err)
	}
	required, err = estimateNVRStorageTransfer(context.Background(), source, destination)
	if err != nil || required != 0 {
		t.Fatalf("idempotent copy estimate %d %v", required, err)
	}
	if err := os.WriteFile(copyPath, []byte("different-archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := copyNVRRecordings(context.Background(), source, destination, os.Getuid(), os.Getgid()); err == nil ||
		!strings.Contains(err.Error(), "conflicting recording") {
		t.Fatalf("modified destination archive overwritten: %v", err)
	}
}

func TestNVRVideoMigrationRejectsSymlinks(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "old")
	destination := filepath.Join(base, "new")
	if err := os.Mkdir(source, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(destination, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(base, "outside")
	if err := os.WriteFile(outside, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(source, "camera.mp4")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := copyNVRRecordings(context.Background(), source, destination, os.Getuid(), os.Getgid()); err == nil {
		t.Fatal("archive symlink accepted")
	}
	if err := safeNVRDiskMount(context.Background(), "/tmp/nvr", "uuid"); err == nil {
		t.Fatal("untrusted video mount point accepted")
	}
	if err := safeNVRDiskMount(context.Background(), "/mnt/home-ai-core/disk", "../bad"); err == nil {
		t.Fatal("malformed device UUID accepted")
	}
}

func TestNVRStorageImageReferenceIsImmutable(t *testing.T) {
	id := "sha256:" + strings.Repeat("a", 64)
	image := "ghcr.io/deadsoulf/home-ai-core_dvr@sha256:" + strings.Repeat("b", 64)
	info := nvrContainerMetadata{Image: id}
	info.Config.Image = image
	if got := validatedNVRImageReference(info); got != image {
		t.Fatalf("immutable NVR digest not retained: %q", got)
	}
	info.Config.Image = "latest"
	if got := validatedNVRImageReference(info); got != id {
		t.Fatalf("mutable tag chosen: %q", got)
	}
}
