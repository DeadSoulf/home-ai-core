package api

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/modules"
	"github.com/DeadSoulf/home-ai-core/internal/nvr"
	"github.com/DeadSoulf/home-ai-core/internal/realtime"
	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/state"
)

type apiLiveSource struct{}

func (apiLiveSource) Available() bool { return true }

func (apiLiveSource) Start(context.Context, nvr.ProbeRequest) (io.ReadCloser, <-chan error, error) {
	done := make(chan error, 1)
	done <- nil
	close(done)
	return io.NopCloser(bytes.NewReader([]byte{0xff, 0xd8, 'o', 'k', 0xff, 0xd9})), done, nil
}

func TestNVRCameraLiveMJPEGRequiresExactLivePermission(t *testing.T) {
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
		"rtsp://192.0.2.91/live",
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

	registry := modules.NewRegistry(store)
	if err := registry.Register(ctx, nvr.NewModule()); err != nil {
		t.Fatal(err)
	}
	if err := registry.SetStatus(ctx, nvr.ModuleID, "enabled", ""); err != nil {
		t.Fatal(err)
	}

	service := nvr.NewServiceWithRuntimeDependencies(
		store,
		&apiNVRSecrets{},
		apiNVRProber{},
		apiLiveSource{},
	)
	sec := defaultFakeSecurity()
	sec.actor.Permissions = []string{"security.self.read"}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	const nodeID = "00000000-0000-4000-8000-000000000000"
	handler := NewWithNVR(
		nodeID,
		logger,
		store,
		sec,
		nil,
		nil,
		registry,
		nil,
		realtime.New(nodeID, logger),
		service,
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nvr/cameras/"+camera.ID+"/live.mjpeg", nil)
	req.Header.Set("Authorization", "Bearer test")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("live without grant status = %d: %s", rec.Code, rec.Body.String())
	}

	sec.actor.ResourcePermissions = []security.PermissionScope{{
		Permission:   nvr.PermissionCameraLive,
		ResourceType: "camera",
		ResourceID:   camera.ID,
	}}
	handler = NewWithNVR(
		nodeID,
		logger,
		store,
		sec,
		nil,
		nil,
		registry,
		nil,
		realtime.New(nodeID, logger),
		service,
	)

	req = httptest.NewRequest(http.MethodGet, "/api/v1/nvr/cameras/"+camera.ID+"/live.mjpeg", nil)
	req.Header.Set("Authorization", "Bearer test")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("live with grant status = %d: %s", rec.Code, rec.Body.String())
	}
	if contentType := rec.Header().Get("Content-Type"); !strings.Contains(contentType, "multipart/x-mixed-replace") {
		t.Fatalf("live content type = %q", contentType)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte{0xff, 0xd8, 'o', 'k', 0xff, 0xd9}) {
		t.Fatalf("live response missing JPEG frame: %v", rec.Body.Bytes())
	}
}
