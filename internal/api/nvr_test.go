package api

import (
	"context"
	"encoding/json"
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

func TestNVRCameraListDoesNotExposeSourceOrCredentialReference(t *testing.T) {
	ctx := context.Background()
	store, err := state.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	_, err = store.CreateNVRCamera(
		ctx,
		"Driveway",
		"rtsp",
		"rtsp://192.0.2.10/private-stream",
		"sec_driveway",
		"tcp",
		"continuous",
		"",
		true,
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

	sec := defaultFakeSecurity()
	sec.actor.Permissions = append(sec.actor.Permissions, nvr.PermissionCameraList)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	const nodeID = "00000000-0000-4000-8000-000000000000"
	handler := New(
		nodeID,
		logger,
		store,
		sec,
		nil,
		nil,
		registry,
		nil,
		realtime.New(nodeID, logger),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nvr/cameras", nil)
	req.Header.Set("Authorization", "Bearer test")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("camera list status = %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "192.0.2.10") || strings.Contains(rec.Body.String(), "sec_driveway") {
		t.Fatalf("camera list leaked source/credential data: %s", rec.Body.String())
	}
	var body struct {
		Cameras []nvr.CameraSummary `json:"cameras"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Cameras) != 1 || !body.Cameras[0].HasCredentials {
		t.Fatalf("camera list = %#v", body.Cameras)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/nvr/status", nil)
	req.Header.Set("Authorization", "Bearer test")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("NVR status = %d: %s", rec.Code, rec.Body.String())
	}
	var statusBody struct {
		NVR nvr.Status `json:"nvr"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &statusBody); err != nil {
		t.Fatal(err)
	}
	if statusBody.NVR.CameraCount != 1 || statusBody.NVR.FoundationStage != "nvr-0" {
		t.Fatalf("NVR status = %#v", statusBody.NVR)
	}
	if statusBody.NVR.MediaRuntimeReady || statusBody.NVR.SecretStoreReady {
		t.Fatalf("NVR-0 incorrectly reports unfinished runtime ready: %#v", statusBody.NVR)
	}
}

func TestNVRCameraListHonorsScopedCameraAccess(t *testing.T) {
	ctx := context.Background()
	store, err := state.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	first, err := store.CreateNVRCamera(
		ctx, "Front", "rtsp", "rtsp://192.0.2.11/front", "", "tcp", "off", "", false, time.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.CreateNVRCamera(
		ctx, "Back", "rtsp", "rtsp://192.0.2.12/back", "", "tcp", "off", "", false, time.Now(),
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

	sec := defaultFakeSecurity()
	sec.actor.Permissions = []string{"security.self.read"}
	sec.actor.ResourcePermissions = []security.PermissionScope{{
		Permission:   nvr.PermissionCameraLive,
		ResourceType: "camera",
		ResourceID:   first.ID,
	}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	const nodeID = "00000000-0000-4000-8000-000000000000"
	handler := New(
		nodeID,
		logger,
		store,
		sec,
		nil,
		nil,
		registry,
		nil,
		realtime.New(nodeID, logger),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nvr/cameras", nil)
	req.Header.Set("Authorization", "Bearer test")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("camera list status = %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Cameras []nvr.CameraSummary `json:"cameras"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Cameras) != 1 || body.Cameras[0].ID != first.ID {
		t.Fatalf("scoped camera list = %#v", body.Cameras)
	}
}

func TestNVRModuleControlAndNavigation(t *testing.T) {
	ctx := context.Background()
	store, err := state.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	registry := modules.NewRegistry(store)
	if err := registry.Register(ctx, nvr.NewModule()); err != nil {
		t.Fatal(err)
	}
	if err := registry.SetStatus(ctx, nvr.ModuleID, "disabled", ""); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	const nodeID = "00000000-0000-4000-8000-000000000000"
	handler := New(
		nodeID,
		logger,
		store,
		defaultFakeSecurity(),
		nil,
		nil,
		registry,
		nil,
		realtime.New(nodeID, logger),
	)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/modules/nvr/control",
		strings.NewReader(`{"operation":"enable"}`),
	)
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("NVR enable status = %d: %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/modules/navigation", nil)
	req.Header.Set("Authorization", "Bearer test")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"/modules/nvr"`) {
		t.Fatalf("NVR navigation = %d: %s", rec.Code, rec.Body.String())
	}
}
