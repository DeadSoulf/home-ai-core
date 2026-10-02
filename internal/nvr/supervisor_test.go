package nvr

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/state"
)

type supervisorProber struct {
	mu     sync.Mutex
	calls  int
	result ProbeResult
	failAt int
}

func (p *supervisorProber) Available() bool { return true }

func (p *supervisorProber) Probe(_ context.Context, _ ProbeRequest) (ProbeResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	if p.failAt > 0 && p.calls >= p.failAt {
		return ProbeResult{}, ErrRTSPConnection
	}
	return p.result, nil
}

func (p *supervisorProber) Calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func TestCameraSupervisorTracksOnlineOfflineAndStops(t *testing.T) {
	ctx := context.Background()
	store, err := state.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	camera, err := store.CreateNVRCamera(
		ctx,
		"Driveway",
		"rtsp",
		"rtsp://192.0.2.50/stream",
		"",
		"tcp",
		"off",
		"",
		false,
		time.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}

	prober := &supervisorProber{
		result: ProbeResult{Codec: "h264", Width: 1920, Height: 1080, FPS: 25},
		failAt: 2,
	}
	service := NewServiceWithDependencies(store, newFakeCredentialStore(), prober)
	service.healthInterval = 5 * time.Millisecond
	service.retryDelays = []time.Duration{5 * time.Millisecond}

	var mu sync.Mutex
	var transitions []RuntimeEvent
	service.SetRuntimeEventHandler(func(event RuntimeEvent) {
		mu.Lock()
		transitions = append(transitions, event)
		mu.Unlock()
	})

	if err := service.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer service.Stop()

	waitForNVRRuntime(t, time.Second, func() bool {
		return service.CameraRuntime(camera.ID).State == RuntimeOnline
	})
	waitForNVRRuntime(t, time.Second, func() bool {
		return service.CameraRuntime(camera.ID).State == RuntimeOffline
	})

	status := service.CameraRuntime(camera.ID)
	if status.LastSeenAt == nil || status.LastCheckedAt == nil {
		t.Fatalf("runtime timestamps = %#v", status)
	}
	if status.LastError != "camera connection failed" {
		t.Fatalf("last error = %q", status.LastError)
	}
	if status.ReconnectCount < 1 {
		t.Fatalf("reconnect count = %d", status.ReconnectCount)
	}
	if prober.Calls() < 2 {
		t.Fatalf("probe calls = %d", prober.Calls())
	}

	mu.Lock()
	seenOnline, seenOffline := false, false
	for _, event := range transitions {
		seenOnline = seenOnline || event.State == RuntimeOnline
		seenOffline = seenOffline || event.State == RuntimeOffline
	}
	mu.Unlock()
	if !seenOnline || !seenOffline {
		t.Fatalf("transitions = %#v", transitions)
	}

	service.Stop()
	if service.SupervisorRunning() {
		t.Fatal("supervisor still running after Stop")
	}
	if state := service.CameraRuntime(camera.ID).State; state != RuntimeDisabled {
		t.Fatalf("camera state after stop = %q", state)
	}
}

func TestCameraSupervisorSkipsDisabledCamera(t *testing.T) {
	ctx := context.Background()
	store, err := state.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	camera, err := store.CreateNVRCamera(
		ctx, "Disabled", "rtsp", "rtsp://192.0.2.51/stream", "", "tcp", "off", "", false, time.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.UpdateNVRCamera(
		ctx,
		camera.ID,
		camera.Name,
		camera.SourceType,
		camera.Address,
		camera.CredentialRef,
		camera.Transport,
		camera.RecordingMode,
		false,
		camera.AudioEnabled,
		time.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}

	prober := &supervisorProber{result: ProbeResult{Codec: "h264"}}
	service := NewServiceWithDependencies(store, newFakeCredentialStore(), prober)
	if err := service.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer service.Stop()

	time.Sleep(20 * time.Millisecond)
	if prober.Calls() != 0 {
		t.Fatalf("disabled camera was probed %d times", prober.Calls())
	}
	if got := service.CameraRuntime(camera.ID).State; got != RuntimeDisabled {
		t.Fatalf("disabled camera runtime = %q", got)
	}
}

func TestRuntimeErrorMessageDoesNotExposeUnknownProviderText(t *testing.T) {
	if got := runtimeErrorMessage(errors.New("rtsp://user:secret@example.invalid/private")); got != "camera health check failed" {
		t.Fatalf("runtime error = %q", got)
	}
}

func waitForNVRRuntime(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("timed out waiting for NVR runtime state")
}
