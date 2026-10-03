package nvr

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/state"
)

type fakeMountChecker struct {
	mounted bool
	err     error
}

func (f fakeMountChecker) Mounted(string) (bool, error) {
	return f.mounted, f.err
}

type fakeRecorderSource struct {
	started chan string
}

func (f *fakeRecorderSource) Available() bool { return true }

func (f *fakeRecorderSource) Start(
	ctx context.Context,
	_ ProbeRequest,
	outputDir string,
	_ bool,
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
	service.mountChecker = fakeMountChecker{mounted: true}
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
			if status.LastSegmentPath != "" {
				return
			}
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

func TestFinalizeRecordedSegmentRenamesPartialFile(t *testing.T) {
	root := t.TempDir()
	partial := filepath.Join(root, "segment.partial.mp4")
	if err := os.WriteFile(partial, []byte("video"), 0o640); err != nil {
		t.Fatal(err)
	}

	finalPath, err := finalizeRecordedSegment(partial)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(finalPath) != "segment.mp4" {
		t.Fatalf("final path = %q", finalPath)
	}
	if _, err := os.Stat(partial); !os.IsNotExist(err) {
		t.Fatalf("partial segment still exists: %v", err)
	}
	if _, err := os.Stat(finalPath); err != nil {
		t.Fatalf("final segment missing: %v", err)
	}
}

func TestCleanupPartialSegmentsPreservesCompletedArchive(t *testing.T) {
	root := t.TempDir()
	cameraRoot := filepath.Join(root, "home-ai-nvr", "cam_test", "session")
	if err := os.MkdirAll(cameraRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	partial := filepath.Join(cameraRoot, "broken.partial.mp4")
	complete := filepath.Join(cameraRoot, "complete.mp4")
	if err := os.WriteFile(partial, []byte("partial"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(complete, []byte("complete"), 0o640); err != nil {
		t.Fatal(err)
	}

	if err := cleanupPartialSegments(filepath.Join(root, "home-ai-nvr", "cam_test")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(partial); !os.IsNotExist(err) {
		t.Fatalf("partial segment survived recovery cleanup: %v", err)
	}
	if _, err := os.Stat(complete); err != nil {
		t.Fatalf("completed segment was removed: %v", err)
	}
}

type retryRecorderSource struct {
	mu    sync.Mutex
	calls int
}

func (r *retryRecorderSource) Available() bool { return true }

func (r *retryRecorderSource) Start(
	ctx context.Context,
	_ ProbeRequest,
	outputDir string,
	_ bool,
) (<-chan RecordedSegment, <-chan error, error) {
	r.mu.Lock()
	r.calls++
	call := r.calls
	r.mu.Unlock()

	segments := make(chan RecordedSegment, 1)
	done := make(chan error, 1)
	if call == 1 {
		close(segments)
		done <- errors.New("simulated RTSP loss")
		close(done)
		return segments, done, nil
	}

	if err := os.MkdirAll(outputDir, 0o750); err != nil {
		return nil, nil, err
	}
	path := filepath.Join(outputDir, "recovered.mp4")
	if err := os.WriteFile(path, []byte("recovered-segment"), 0o640); err != nil {
		return nil, nil, err
	}
	segments <- RecordedSegment{Path: path, EndOffset: time.Minute}
	close(segments)
	go func() {
		<-ctx.Done()
		done <- ctx.Err()
		close(done)
	}()
	return segments, done, nil
}

func (r *retryRecorderSource) Calls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func TestContinuousRecordingRestartsAfterRecorderProcessLoss(t *testing.T) {
	ctx := context.Background()
	store, err := state.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	mountpoint := t.TempDir()
	target, err := store.SetNVRStorageTarget(
		ctx, "/dev/retry-video", "uuid-retry-video", mountpoint, 10, true, time.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}

	recorder := &retryRecorderSource{}
	service := NewServiceWithDependencies(
		store,
		newFakeCredentialStore(),
		&fakeProber{result: ProbeResult{Codec: "h264", Width: 1920, Height: 1080}},
	)
	service.recorder = recorder
	service.mountChecker = fakeMountChecker{mounted: true}
	service.retryDelays = []time.Duration{time.Millisecond}
	service.healthInterval = time.Hour

	camera, _, err := service.CreateCamera(ctx, "", CameraInput{
		Name:          "Reconnect recorder",
		Address:       "rtsp://192.0.2.120/main",
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
		if recorder.Calls() >= 2 && total == int64(len("recovered-segment")) {
			status := service.CameraRecording(camera.ID)
			if !status.Active || status.LastSegmentPath == "" {
				t.Fatalf("recording status after reconnect = %#v", status)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("recorder did not recover: calls=%d status=%#v", recorder.Calls(), service.CameraRecording(camera.ID))
}

func TestContinuousRecordingRejectsUnmountedArchiveTarget(t *testing.T) {
	ctx := context.Background()
	store, err := state.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	mountpoint := t.TempDir()
	if _, err := store.SetNVRStorageTarget(
		ctx, "/dev/unmounted-video", "uuid-unmounted-video", mountpoint, 10, true, time.Now(),
	); err != nil {
		t.Fatal(err)
	}

	service := NewServiceWithDependencies(
		store,
		newFakeCredentialStore(),
		&fakeProber{result: ProbeResult{Codec: "h264"}},
	)
	service.recorder = &fakeRecorderSource{}
	service.mountChecker = fakeMountChecker{mounted: false}

	camera, _, err := service.CreateCamera(ctx, "", CameraInput{
		Name:          "Unmounted archive",
		Address:       "rtsp://192.0.2.121/main",
		Transport:     "tcp",
		RecordingMode: "continuous",
	})
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.NVRCamera(ctx, camera.ID)
	if err != nil {
		t.Fatal(err)
	}

	if err := service.startRecording(ctx, record); !errors.Is(err, ErrRecordingStorageUnmounted) {
		t.Fatalf("start recording error = %v, want %v", err, ErrRecordingStorageUnmounted)
	}
}
