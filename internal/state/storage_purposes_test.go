package state

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestStoragePurposeRoundTripAndUUIDRebind(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	now := time.Date(2026, 9, 30, 20, 0, 0, 0, time.UTC)
	record, err := store.SetStoragePurpose(ctx, "/dev/sdb1", "uuid-1", StoragePurposeFiles, now)
	if err != nil {
		t.Fatal(err)
	}
	if record.DevicePath != "/dev/sdb1" || record.FilesystemUUID != "uuid-1" || record.Purpose != StoragePurposeFiles {
		t.Fatalf("unexpected record: %#v", record)
	}

	updated, err := store.SetStoragePurpose(ctx, "/dev/nvme1n1p1", "uuid-1", StoragePurposeVideo, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if updated.DevicePath != "/dev/nvme1n1p1" || updated.Purpose != StoragePurposeVideo {
		t.Fatalf("unexpected rebound record: %#v", updated)
	}

	records, err := store.ListStoragePurposes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1: %#v", len(records), records)
	}
	if records[0].DevicePath != "/dev/nvme1n1p1" {
		t.Fatalf("device path = %q", records[0].DevicePath)
	}

	if err := store.ClearStoragePurpose(ctx, "/dev/nvme1n1p1", "uuid-1"); err != nil {
		t.Fatal(err)
	}
	records, err = store.ListStoragePurposes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Fatalf("records after clear = %#v", records)
	}
}

func TestStoragePurposeRejectsInvalidPurpose(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if _, err := store.SetStoragePurpose(ctx, "/dev/sdb1", "", "backup", time.Now()); err == nil {
		t.Fatal("invalid storage purpose was accepted")
	}
}
