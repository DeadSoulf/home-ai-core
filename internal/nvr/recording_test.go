package nvr

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/state"
)

type fakeRecorderSource struct {
	started chan string
}

func (f *fakeRecorderSource) Available() bool { return true }

func (f *fakeRecorderSource) Start(
	ctx context.Context,
	_ ProbeRequest,
	outputDir string,
) (<-chan RecordedSegment, <-chan error, error) {
	if err := os.MkdirAll(outputDir, 0o750); err != nil {
		return nil, nil, err
	}
	path := filepath.Join(outputDir, "segment.mp4")
	if err := os.WriteFile(path, []byte("segment-data"), 0o640); err != nil {
		return nil, nil, err
	}
	segments := make(chan RecordedSegment, 1)
	done := make(chan error, 1)
	segments <- RecordedSegment{
		Path:        path,
		StartOffset: 0,
		EndOffset:   time.Minute,
	}
	close(segments)
	if f.started != nil {
		f.started <- outputDir
	}
	go func() {
		<-ctx.Done()
		done <- ctx.Err()
		close(done)
	}()
	return segments, done, nil
}

func TestContinuousRecordingIndexesCompletedSegment(t *testing.T) {
	ctx := context.Background()
	store, err := state.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	mountpoint := t.TempDir()
	target, err := store.SetNVRStorageTarget(
		ctx, "/dev/test-video", "uuid-test-video", mountpoint, 10, true, time.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}

	prober := &fakeProber{result: ProbeResult{
		Codec: "h264", Width: 1920, Height: 1080, FPS: 25, BitrateBPS: 4_000_000,
	}}
	service := NewServiceWithDependencies(store, newFakeCredentialStore(), prober)
	service.recorder = &fakeRecorderSource{started: make(chan string, 1)}
	service.healthInterval = time.Hour

	camera, _, err := service.CreateCamera(ctx, "", CameraInput{
		Name:          "Recorder test",
		Address:       "rtsp://192.0.2.100/main",
		Transport:     "tcp",
		RecordingMode: "continuous",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer service.Stop()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		total, err := store.NVRArchiveBytes(ctx, target.ID)
		if err != nil {
			t.Fatal(err)
		}
		if total == int64(len("segment-data")) {
			status := service.CameraRecording(camera.ID)
			if status.LastSegmentPath == "" {
				t.Fatalf("recording status = %#v", status)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("recording segment was not indexed")
}

func TestContinuousRecordingWithoutTargetDoesNotFailNVRStart(t *testing.T) {
	ctx := context.Background()
	store, err := state.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	service := NewServiceWithDependencies(
		store,
		newFakeCredentialStore(),
		&fakeProber{result: ProbeResult{Codec: "h264"}},
	)
	service.recorder = &fakeRecorderSource{}
	service.healthInterval = time.Hour

	camera, _, err := service.CreateCamera(ctx, "", CameraInput{
		Name:          "No target",
		Address:       "rtsp://192.0.2.101/main",
		Transport:     "tcp",
		RecordingMode: "continuous",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Start(ctx); err != nil {
		t.Fatalf("NVR start failed because recording storage is absent: %v", err)
	}
	defer service.Stop()

	status := service.CameraRecording(camera.ID)
	if status.LastError != "recording storage is not configured" {
		t.Fatalf("recording status = %#v", status)
	}
}


type fakeSpaceChecker struct {
	values []FilesystemSpace
	index  int
}

func (f *fakeSpaceChecker) Space(string) (FilesystemSpace, error) {
	if len(f.values) == 0 {
		return FilesystemSpace{}, nil
	}
	if f.index >= len(f.values) {
		return f.values[len(f.values)-1], nil
	}
	value := f.values[f.index]
	f.index++
	return value, nil
}

func TestRingRetentionDeletesOldestUnprotectedSegmentOnly(t *testing.T) {
	ctx := context.Background()
	store, err := state.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	mountpoint := t.TempDir()
	target, err := store.SetNVRStorageTarget(
		ctx, "/dev/video", "uuid-video", mountpoint, 10, true, time.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}
	camera, err := store.CreateNVRCamera(
		ctx, "Retention camera", "rtsp", "rtsp://192.0.2.110/main", "",
		"tcp", "continuous", "", false, time.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}

	oldPath := filepath.Join(mountpoint, "home-ai-nvr", camera.ID, "old.mp4")
	protectedPath := filepath.Join(mountpoint, "home-ai-nvr", camera.ID, "protected.mp4")
	if err := os.MkdirAll(filepath.Dir(oldPath), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldPath, []byte("old"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(protectedPath, []byte("protected"), 0o640); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	_, err = store.CreateNVRRecordingSegment(
		ctx, camera.ID, target.ID, now.Add(-2*time.Minute), now.Add(-time.Minute),
		filepath.ToSlash(strings.TrimPrefix(oldPath, mountpoint+string(filepath.Separator))),
		"h264", 1920, 1080, 3, false, "complete", now,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.CreateNVRRecordingSegment(
		ctx, camera.ID, target.ID, now.Add(-time.Minute), now,
		filepath.ToSlash(strings.TrimPrefix(protectedPath, mountpoint+string(filepath.Separator))),
		"h264", 1920, 1080, 9, true, "complete", now,
	)
	if err != nil {
		t.Fatal(err)
	}

	service := NewServiceWithDependencies(
		store, newFakeCredentialStore(), &fakeProber{result: ProbeResult{Codec: "h264"}},
	)
	service.spaceChecker = &fakeSpaceChecker{values: []FilesystemSpace{
		{TotalBytes: 1000, AvailableBytes: 50},
		{TotalBytes: 1000, AvailableBytes: 150},
	}}

	if err := service.enforceStorageReserve(ctx, target); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old segment still exists: %v", err)
	}
	if _, err := os.Stat(protectedPath); err != nil {
		t.Fatalf("protected segment was removed: %v", err)
	}
	total, err := store.NVRArchiveBytes(ctx, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if total != 9 {
		t.Fatalf("archive bytes = %d, want protected segment only", total)
	}
}

func TestRingRetentionStopsWhenOnlyProtectedSegmentsRemain(t *testing.T) {
	ctx := context.Background()
	store, err := state.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	mountpoint := t.TempDir()
	target, err := store.SetNVRStorageTarget(
		ctx, "/dev/video", "uuid-video", mountpoint, 10, true, time.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}
	camera, err := store.CreateNVRCamera(
		ctx, "Protected camera", "rtsp", "rtsp://192.0.2.111/main", "",
		"tcp", "continuous", "", false, time.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}
	protectedPath := filepath.Join(mountpoint, "home-ai-nvr", camera.ID, "protected.mp4")
	if err := os.MkdirAll(filepath.Dir(protectedPath), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(protectedPath, []byte("protected"), 0o640); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	_, err = store.CreateNVRRecordingSegment(
		ctx, camera.ID, target.ID, now.Add(-time.Minute), now,
		filepath.ToSlash(strings.TrimPrefix(protectedPath, mountpoint+string(filepath.Separator))),
		"h264", 1920, 1080, 9, true, "complete", now,
	)
	if err != nil {
		t.Fatal(err)
	}

	service := NewServiceWithDependencies(
		store, newFakeCredentialStore(), &fakeProber{result: ProbeResult{Codec: "h264"}},
	)
	service.spaceChecker = &fakeSpaceChecker{values: []FilesystemSpace{
		{TotalBytes: 1000, AvailableBytes: 50},
	}}

	if err := service.enforceStorageReserve(ctx, target); err != ErrRecordingStorageFull {
		t.Fatalf("retention error = %v, want %v", err, ErrRecordingStorageFull)
	}
	if _, err := os.Stat(protectedPath); err != nil {
		t.Fatalf("protected segment was removed: %v", err)
	}
}
