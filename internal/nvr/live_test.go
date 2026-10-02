package nvr

import (
	"bufio"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/state"
)

func TestFFmpegMJPEGSourceKeepsCredentialsOutOfArguments(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "ffmpeg")
	argsFile := filepath.Join(dir, "args.txt")
	stdinFile := filepath.Join(dir, "stdin.txt")
	content := "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" > " + argsFile + "\n" +
		"cat > " + stdinFile + "\n" +
		"printf '\\377\\330abc\\377\\331\\377\\330def\\377\\331'\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}

	source := &FFmpegMJPEGSource{path: script}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stdout, done, err := source.Start(ctx, ProbeRequest{
		Address:   "rtsp://192.0.2.77/live",
		Transport: "tcp",
		Credential: CameraCredential{
			Username: "viewer",
			Password: "camera-secret",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stdout.Close()

	reader := bufio.NewReader(stdout)
	first, err := readJPEGFrame(reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	second, err := readJPEGFrame(reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string([]byte{0xff, 0xd8, 'a', 'b', 'c', 0xff, 0xd9}) {
		t.Fatalf("first frame = %v", first)
	}
	if string(second) != string([]byte{0xff, 0xd8, 'd', 'e', 'f', 0xff, 0xd9}) {
		t.Fatalf("second frame = %v", second)
	}
	<-done

	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(args), "viewer") || strings.Contains(string(args), "camera-secret") {
		t.Fatalf("ffmpeg argv leaked credentials: %s", args)
	}
	stdin, err := os.ReadFile(stdinFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(stdin), "viewer") || !strings.Contains(string(stdin), "camera-secret") {
		t.Fatalf("ffmpeg stdin did not receive protected source: %s", stdin)
	}
}

type fakeLiveSource struct {
	mu       sync.Mutex
	starts   int
	requests []ProbeRequest
}

func (s *fakeLiveSource) Available() bool { return true }

func (s *fakeLiveSource) Start(ctx context.Context, request ProbeRequest) (io.ReadCloser, <-chan error, error) {
	s.mu.Lock()
	s.starts++
	s.requests = append(s.requests, request)
	s.mu.Unlock()

	reader, writer := io.Pipe()
	done := make(chan error, 1)
	go func() {
		defer writer.Close()
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				done <- ctx.Err()
				close(done)
				return
			case <-ticker.C:
				if _, err := writer.Write([]byte{0xff, 0xd8, 'x', 0xff, 0xd9}); err != nil {
					done <- err
					close(done)
					return
				}
			}
		}
	}()
	return reader, done, nil
}

func (s *fakeLiveSource) Starts() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.starts
}

func (s *fakeLiveSource) LastRequest() ProbeRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.requests) == 0 {
		return ProbeRequest{}
	}
	return s.requests[len(s.requests)-1]
}

func TestLiveSubscribersShareOneCameraProcess(t *testing.T) {
	ctx := context.Background()
	store, err := state.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	camera, err := store.CreateNVRCamera(
		ctx,
		"Front",
		"rtsp",
		"rtsp://192.0.2.88/live",
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

	source := &fakeLiveSource{}
	service := NewServiceWithRuntimeDependencies(
		store,
		newFakeCredentialStore(),
		&supervisorProber{result: ProbeResult{Codec: "h264"}},
		source,
	)
	service.liveIdleTimeout = 5 * time.Millisecond

	first, err := service.SubscribeLive(ctx, camera.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := service.SubscribeLive(ctx, camera.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()

	frameCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if _, err := first.Next(frameCtx); err != nil {
		t.Fatal(err)
	}
	if _, err := second.Next(frameCtx); err != nil {
		t.Fatal(err)
	}
	if source.Starts() != 1 {
		t.Fatalf("live source starts = %d, want 1", source.Starts())
	}

	first.Close()
	second.Close()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if service.ActiveLiveStreams() == 0 {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("shared live stream did not stop after idle timeout")
}

func TestLivePrefersConfiguredSubstream(t *testing.T) {
	ctx := context.Background()
	store, err := state.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	camera, err := store.CreateNVRCamera(
		ctx,
		"Garage",
		"rtsp",
		"rtsp://192.0.2.90/main",
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
	if _, err := store.SetNVRStreamProfile(
		ctx,
		camera.ID,
		"main",
		"rtsp://192.0.2.90/main",
		"h264",
		1920,
		1080,
		25,
		4_000_000,
		time.Now(),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetNVRStreamProfile(
		ctx,
		camera.ID,
		"sub",
		"rtsp://192.0.2.90/sub",
		"h264",
		640,
		360,
		10,
		500_000,
		time.Now(),
	); err != nil {
		t.Fatal(err)
	}

	source := &fakeLiveSource{}
	service := NewServiceWithRuntimeDependencies(
		store,
		newFakeCredentialStore(),
		&supervisorProber{result: ProbeResult{Codec: "h264"}},
		source,
	)
	subscription, err := service.SubscribeLive(ctx, camera.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer subscription.Close()

	if got := source.LastRequest().Address; got != "rtsp://192.0.2.90/sub" {
		t.Fatalf("live address = %q, want substream", got)
	}
}

func TestReadJPEGFrameRejectsOversize(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader(string([]byte{0xff, 0xd8}) + strings.Repeat("x", 32)))
	if _, err := readJPEGFrame(reader, 8); err == nil {
		t.Fatal("oversize frame was accepted")
	}
}
