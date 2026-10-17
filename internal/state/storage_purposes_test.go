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

	updated, err := store.SetStoragePurpose(ctx, "/dev/nvme1n1p1", "uuid-1", StoragePurposeFiles, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if updated.DevicePath != "/dev/nvme1n1p1" || updated.Purpose != StoragePurposeFiles {
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

func TestModuleStoragePurposeSchemaMigrationAndValidation(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, t.TempDir())
	if err != nil { t.Fatal(err) }
	defer db.Close()
	for _, good := range []string{"module:nvr.module", "module:reference.module", "files"} {
		if got, err := NormalizeStoragePurpose(good); err != nil || got != good {
			t.Fatalf("valid purpose %q rejected: %q %v", good, got, err)
		}
	}
	for _, bad := range []string{"module:", "module:UPPERCASE", "module:../bad", "module:test/escape", "module:nvr.module; rm -rf /", "backup"} {
		if _, err := NormalizeStoragePurpose(bad); err == nil {
			t.Fatalf("unsafe module purpose accepted %q", bad)
		}
	}
	record, err := db.SetStoragePurpose(ctx, "/dev/sdz1", "nvr-test-uuid", "module:nvr.module", time.Now())
	if err != nil || record.Purpose != "module:nvr.module" { t.Fatalf("failed module storage assign: %#v %v", record, err) }
	all, err := db.ListStoragePurposes(ctx)
	if err != nil || len(all) != 1 || all[0].Purpose != "module:nvr.module" {
		t.Fatalf("module purpose missing after migration: %+v %v", all, err)
	}
}
