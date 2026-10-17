package state

import (
	"context"
	"testing"
	"time"
)

func TestNVRVideoStoragePurposeAndActivatedRecord(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	for _, purpose := range []string{StoragePurposeFiles, StoragePurposeNVR} {
		got, err := NormalizeStoragePurpose(purpose)
		if err != nil || got != purpose {
			t.Fatalf("normalize %q = %q %v", purpose, got, err)
		}
	}
	if _, err := NormalizeStoragePurpose("everything"); err == nil {
		t.Fatal("unexpected storage purpose accepted")
	}
	assignment, err := store.SetStoragePurpose(ctx, "/dev/sdc1", "uuid-video", StoragePurposeNVR, time.Now())
	if err != nil || assignment.Purpose != StoragePurposeNVR {
		t.Fatalf("assign video partition: %+v %v", assignment, err)
	}
	record, exists, err := store.NVRVideoStorage(ctx)
	if err != nil || exists {
		t.Fatalf("unconfigured video storage should be empty: %+v %v %v", record, exists, err)
	}
	selected := NVRVideoStorage{
		DevicePath: "/dev/sdc1", FilesystemUUID: "uuid-video",
		Mountpoint: "/mnt/home-ai-core/nvr-disk",
	}
	if err := store.SetNVRVideoStorage(ctx, selected); err != nil {
		t.Fatal(err)
	}
	record, exists, err = store.NVRVideoStorage(ctx)
	if err != nil || !exists ||
		record.DevicePath != selected.DevicePath ||
		record.FilesystemUUID != selected.FilesystemUUID ||
		record.Mountpoint != selected.Mountpoint ||
		record.ActiveAt == "" {
		t.Fatalf("persisted video disk invalid: %+v exists=%v err=%v", record, exists, err)
	}
	updated, err := store.SetStoragePurpose(ctx, "/dev/disk/by-uuid/uuid-video", "uuid-video", StoragePurposeNVR, time.Now())
	if err != nil || updated.Purpose != StoragePurposeNVR {
		t.Fatalf("rebind video disk UUID failed: %+v %v", updated, err)
	}
	entries, err := store.ListStoragePurposes(ctx)
	if err != nil || len(entries) != 1 {
		t.Fatalf("duplicate video purpose record: %+v %v", entries, err)
	}
}
