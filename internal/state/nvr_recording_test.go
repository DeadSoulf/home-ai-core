package state

import (
	"context"
	"testing"
	"time"
)

func TestNVRStorageTargetSelectionAndRetentionIndex(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	now := time.Date(2026, 10, 3, 8, 0, 0, 0, time.UTC)
	first, err := store.SetNVRStorageTarget(
		ctx, "/dev/sdb1", "uuid-video-1", "/mnt/video1", 10, true, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Active || first.ReservePercent != 10 {
		t.Fatalf("first target = %#v", first)
	}

	second, err := store.SetNVRStorageTarget(
		ctx, "/dev/sdc1", "uuid-video-2", "/mnt/video2", 5, true, now.Add(time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Active {
		t.Fatalf("second target = %#v", second)
	}
	first, err = store.NVRStorageTarget(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if first.Active {
		t.Fatal("previous storage target remained active")
	}
	active, err := store.ActiveNVRStorageTarget(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if active.ID != second.ID {
		t.Fatalf("active target = %#v", active)
	}

	camera, err := store.CreateNVRCamera(
		ctx, "Archive camera", "rtsp", "rtsp://192.0.2.90/main", "",
		"tcp", "continuous", "", false, now,
	)
	if err != nil {
		t.Fatal(err)
	}

	oldest, err := store.CreateNVRRecordingSegment(
		ctx, camera.ID, second.ID,
		now, now.Add(time.Minute), "cam/oldest.mp4", "h264",
		1920, 1080, 100, false, "complete", now,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.CreateNVRRecordingSegment(
		ctx, camera.ID, second.ID,
		now.Add(time.Minute), now.Add(2*time.Minute), "cam/protected.mp4", "h264",
		1920, 1080, 200, true, "complete", now,
	)
	if err != nil {
		t.Fatal(err)
	}
	newest, err := store.CreateNVRRecordingSegment(
		ctx, camera.ID, second.ID,
		now.Add(2*time.Minute), now.Add(3*time.Minute), "cam/newest.mp4", "h264",
		1920, 1080, 300, false, "complete", now,
	)
	if err != nil {
		t.Fatal(err)
	}

	total, err := store.NVRArchiveBytes(ctx, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if total != 600 {
		t.Fatalf("archive bytes = %d, want 600", total)
	}

	candidates, err := store.OldestNVRRetentionSegments(ctx, second.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 2 {
		t.Fatalf("retention candidates = %#v", candidates)
	}
	if candidates[0].ID != oldest.ID || candidates[1].ID != newest.ID {
		t.Fatalf("retention order = %#v", candidates)
	}
	for _, candidate := range candidates {
		if candidate.Protected {
			t.Fatalf("protected segment selected for retention: %#v", candidate)
		}
	}

	if err := store.DeleteNVRRecordingSegment(ctx, oldest.ID); err != nil {
		t.Fatal(err)
	}
	total, err = store.NVRArchiveBytes(ctx, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if total != 500 {
		t.Fatalf("archive bytes after delete = %d, want 500", total)
	}
}

func TestNVRStorageTargetRejectsUnsafeReserve(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if _, err := store.SetNVRStorageTarget(
		ctx, "/dev/sdb1", "uuid", "/mnt/video", 0, true, time.Now(),
	); err == nil {
		t.Fatal("zero reserve was accepted")
	}
	if _, err := store.SetNVRStorageTarget(
		ctx, "/dev/sdb1", "uuid", "/mnt/video", 51, true, time.Now(),
	); err == nil {
		t.Fatal("reserve above schema maximum was accepted")
	}
}
