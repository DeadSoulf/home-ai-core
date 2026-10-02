package state

import (
	"context"
	"testing"
	"time"
)

func TestNVRFoundationPersistence(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	camera, err := store.CreateNVRCamera(
		ctx,
		"Driveway",
		"rtsp",
		"rtsp://192.0.2.10/stream1",
		"sec_driveway",
		"tcp",
		"continuous",
		"",
		true,
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if camera.ID == "" || camera.Name != "Driveway" || !camera.Enabled {
		t.Fatalf("camera = %#v", camera)
	}
	if camera.CredentialRef != "sec_driveway" {
		t.Fatalf("credential ref = %q", camera.CredentialRef)
	}

	profile, err := store.SetNVRStreamProfile(
		ctx,
		camera.ID,
		"main",
		"rtsp://192.0.2.10/stream1",
		"h264",
		1920,
		1080,
		25,
		4_000_000,
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if profile.Role != "main" || profile.Width != 1920 || profile.BitrateBPS != 4_000_000 {
		t.Fatalf("profile = %#v", profile)
	}

	cameras, err := store.ListNVRCameras(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(cameras) != 1 || cameras[0].ID != camera.ID {
		t.Fatalf("cameras = %#v", cameras)
	}
	profiles, err := store.ListNVRStreamProfiles(ctx, camera.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 || profiles[0].ID != profile.ID {
		t.Fatalf("profiles = %#v", profiles)
	}
}

func TestNVRFoundationSchemaProtectsStorageAndPermissions(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	var count int
	if err := store.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM permissions
		WHERE name IN (
			'camera.list', 'camera.live', 'camera.archive', 'camera.export',
			'camera.ptz', 'camera.manage', 'nvr.storage.manage', 'nvr.settings.manage'
		)
	`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 8 {
		t.Fatalf("NVR permission count = %d, want 8", count)
	}
	if err := store.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM role_permissions
		WHERE role_id = 'role_owner'
		  AND permission_name IN (
			'camera.list', 'camera.live', 'camera.archive', 'camera.export',
			'camera.ptz', 'camera.manage', 'nvr.storage.manage', 'nvr.settings.manage'
		  )
	`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 8 {
		t.Fatalf("owner NVR permission count = %d, want 8", count)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO nvr_storage_targets(
			id, device_path, filesystem_uuid, mountpoint, reserve_percent, active, created_at, updated_at
		) VALUES ('nvt_1', '/dev/sdb1', 'uuid-1', '/mnt/video1', 5, 1, ?, ?)
	`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO nvr_storage_targets(
			id, device_path, filesystem_uuid, mountpoint, reserve_percent, active, created_at, updated_at
		) VALUES ('nvt_2', '/dev/sdc1', 'uuid-2', '/mnt/video2', 5, 1, ?, ?)
	`, now, now); err == nil {
		t.Fatal("second active NVR storage target was accepted")
	}
}

func TestCreateNVRCameraRejectsInvalidCredentialReference(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	_, err = store.CreateNVRCamera(
		ctx,
		"Camera",
		"rtsp",
		"rtsp://192.0.2.1/stream",
		"plain-password",
		"tcp",
		"off",
		"",
		false,
		time.Now(),
	)
	if err == nil {
		t.Fatal("plain credential value was accepted as a secret reference")
	}
}

func TestUpdateAndDeleteNVRCamera(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	camera, err := store.CreateNVRCamera(
		ctx,
		"Front",
		"rtsp",
		"rtsp://192.0.2.30/main",
		"sec_front",
		"tcp",
		"off",
		"",
		false,
		time.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}

	updated, err := store.UpdateNVRCamera(
		ctx,
		camera.ID,
		"Front door",
		"rtsp",
		"rtsp://192.0.2.30/stream2",
		"sec_front",
		"udp",
		"continuous",
		false,
		true,
		time.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "Front door" || updated.Enabled || !updated.AudioEnabled {
		t.Fatalf("updated camera = %#v", updated)
	}
	if updated.Transport != "udp" || updated.RecordingMode != "continuous" {
		t.Fatalf("updated transport/mode = %#v", updated)
	}

	if err := store.DeleteNVRCamera(ctx, camera.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.NVRCamera(ctx, camera.ID); err != ErrNVRCameraNotFound {
		t.Fatalf("camera after delete error = %v", err)
	}
}
