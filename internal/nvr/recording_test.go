package nvr

import (
	"context"
	"os"
	"path/filepath"
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
