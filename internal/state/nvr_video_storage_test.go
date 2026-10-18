package state

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNVRVideoStoragePurposeAndFilesIsolation(t *testing.T) {
	for _, candidate := range []string{"nvr","NVR"," nvr "} {
		got, err := NormalizeStoragePurpose(candidate)
		if err != nil || got != StoragePurposeNVR {
			t.Fatalf("normalize %q: %q %v", candidate, got, err)
		}
	}
	if _, err := NormalizeStoragePurpose("camera-admin"); err == nil {
		t.Fatal("untrusted storage purpose accepted")
	}
	ctx := context.Background()
	store, err := Open(ctx, t.TempDir())
	if err != nil { t.Fatal(err) }
	defer store.Close()
	now := time.Now().UTC()
	if _, err := store.SetStoragePurpose(ctx, "/dev/sdb1", "filesystem-files", StoragePurposeFiles, now); err != nil { t.Fatal(err) }
	if _, err := store.SetStoragePurpose(ctx, "/dev/sdc1", "filesystem-nvr", StoragePurposeNVR, now); err != nil { t.Fatal(err) }
	rows, err := store.ListStoragePurposes(ctx)
	if err != nil { t.Fatal(err) }
	if len(rows) != 2 || rows[0].Purpose != StoragePurposeFiles || rows[1].Purpose != StoragePurposeNVR {
		t.Fatalf("storage purposes unexpectedly mixed: %+v", rows)
	}
}

func TestNVRPurposeMigrationPreservesExistingFilesDisks(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, t.TempDir())
	if err != nil { t.Fatal(err) }
	defer store.Close()
	// Recreate the pre-upgrade purpose schema then reapply the exact
	// migration. It must keep device identity, timestamps and indexes.
	oldSchema, err := os.ReadFile(filepath.Join("migrations", "013_storage_purposes.sql"))
	if err != nil { t.Fatal(err) }
	if _, err := store.db.ExecContext(ctx, "DROP TABLE storage_purposes"); err != nil { t.Fatal(err) }
	if _, err := store.db.ExecContext(ctx, string(oldSchema)); err != nil { t.Fatal(err) }
	const stamp = "2026-10-10T01:01:02Z"
	if _, err := store.db.ExecContext(ctx,
		"INSERT INTO storage_purposes(device_path,filesystem_uuid,purpose,created_at,updated_at) VALUES (?,?,?,?,?)",
		"/dev/sdb1", "persisting-files-uuid", "files", stamp, stamp); err != nil { t.Fatal(err) }
	migration, err := os.ReadFile(filepath.Join("migrations", "027_nvr_video_disk_purpose.sql"))
	if err != nil { t.Fatal(err) }
	if _, err := store.db.ExecContext(ctx, string(migration)); err != nil { t.Fatal(err) }
	rows, err := store.ListStoragePurposes(ctx)
	if err != nil { t.Fatal(err) }
	if len(rows) != 1 || rows[0].DevicePath != "/dev/sdb1" ||
		rows[0].FilesystemUUID != "persisting-files-uuid" ||
		rows[0].Purpose != "files" || !strings.HasPrefix(rows[0].CreatedAt.Format(time.RFC3339), "2026-10-10") {
		t.Fatalf("migration lost Files configuration: %+v", rows)
	}
	if _, err := store.SetStoragePurpose(ctx, "/dev/sdc1", "nvr-uuid", StoragePurposeNVR, time.Now()); err != nil {
		t.Fatalf("new NVR purpose rejected: %v", err)
	}
}
