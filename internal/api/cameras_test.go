package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DeadSoulf/home-ai-core/internal/cameras"
	"github.com/DeadSoulf/home-ai-core/internal/modules"
	"github.com/DeadSoulf/home-ai-core/internal/realtime"
	"github.com/DeadSoulf/home-ai-core/internal/state"
)

type fakeCamerasService struct {
	status  cameras.Status
	last    cameras.LoginRequest
	refresh int
}

func (f *fakeCamerasService) Status() cameras.Status {
	return f.status
}

func (f *fakeCamerasService) Refresh() cameras.Status {
	f.refresh++
	return f.status
}

func (f *fakeCamerasService) TestLogin(
	_ context.Context,
	request cameras.LoginRequest,
) (cameras.LoginResult, error) {
	f.last = request
	return cameras.LoginResult{
		OK:      true,
		Address: request.Address,
		Port:    request.Port,
		Backend: "HCNetSDK",
		Device: &cameras.DeviceMetadata{
			SerialNumber:   "DS-TEST-001",
			DeviceTypeName: "DS-7608NI",
			Firmware:       "V5.7.18 build 20241004",
			IPChannelCount: 8,
			StartIPChannel: 33,
			Channels:       []cameras.DeviceChannel{{Number: 33, Kind: "ip"}},
		},
	}, nil
}

func (f *fakeCamerasService) ProbeWebSDK(
	_ context.Context,
	request cameras.WebSDKProbeRequest,
) (cameras.WebSDKProbeResult, error) {
	return cameras.WebSDKProbeResult{
		OK:      true,
		Address: request.Address,
		Port:    request.Port,
		HTTPS:   request.HTTPS,
		Backend: "HCWebSDK/ISAPI",
		Device: cameras.WebSDKDeviceInfo{
			DeviceName:      "Front NVR",
			Model:           "DS-7608NI-K2",
			SerialNumber:    "WEBSDK-TEST-001",
			FirmwareVersion: "V4.72.109",
		},
		Ports: cameras.WebSDKPortInfo{HTTPPort: 80, RTSPPort: 554, DevicePort: 8000},
		Channels: []cameras.WebSDKChannel{
			{ID: "1", Kind: "digital", Name: "Gate"},
		},
	}, nil
}

func TestCamerasSDKStatusAndLoginProbe(t *testing.T) {
	ctx := context.Background()
	store, err := state.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	registry := modules.NewRegistry(store)
	if err := registry.Register(ctx, cameras.NewModule()); err != nil {
		t.Fatal(err)
	}
	if err := registry.SetStatus(ctx, cameras.ModuleID, "enabled", ""); err != nil {
		t.Fatal(err)
	}

	service := &fakeCamerasService{status: cameras.Status{
		ModuleID: cameras.ModuleID,
		Version:  cameras.ModuleVersion,
		Backend:  "HCNetSDK",
		SDK: cameras.SDKStatus{
			Architecture: "amd64",
			Supported:    true,
			Available:    true,
			Initialized:  true,
			LibraryPath:  "/opt/home-ai/hikvision/lib/libhcnetsdk.so",
		},
	}}

	sec := defaultFakeSecurity()
	sec.actor.Permissions = append(sec.actor.Permissions, "camera.manage")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	const nodeID = "00000000-0000-4000-8000-000000000000"
	handler := NewWithCameras(
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

	req := httptest.NewRequest(http.MethodGet, "/api/v1/cameras/status", nil)
	req.Header.Set("Authorization", "Bearer test")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status response = %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "libhcnetsdk.so") {
		t.Fatalf("SDK status missing library path: %s", rec.Body.String())
	}

	req = httptest.NewRequest(
		http.MethodPost,
		"/api/v1/cameras/test-login",
		strings.NewReader(`{"address":"192.168.1.64","port":8000,"username":"admin","password":"secret-camera-password"}`),
	)
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login probe response = %d: %s", rec.Code, rec.Body.String())
	}
	if service.last.Address != "192.168.1.64" || service.last.Port != 8000 {
		t.Fatalf("login request = %#v", service.last)
	}
	if strings.Contains(rec.Body.String(), "secret-camera-password") {
		t.Fatalf("camera password leaked in response: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "DS-TEST-001") || !strings.Contains(rec.Body.String(), "DS-7608NI") {
		t.Fatalf("camera metadata missing in response: %s", rec.Body.String())
	}

	req = httptest.NewRequest(
		http.MethodPost,
		"/api/v1/cameras/websdk/probe",
		strings.NewReader(`{"address":"192.168.1.64","port":80,"username":"admin","password":"secret-web-password"}`),
	)
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("WebSDK probe response = %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "WEBSDK-TEST-001") || !strings.Contains(rec.Body.String(), "DS-7608NI-K2") {
		t.Fatalf("WebSDK metadata missing in response: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "secret-web-password") {
		t.Fatalf("WebSDK camera password leaked in response: %s", rec.Body.String())
	}
}

func TestCamerasModuleControlPublishesNavigation(t *testing.T) {
	ctx := context.Background()
	store, err := state.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	registry := modules.NewRegistry(store)
	if err := registry.Register(ctx, cameras.NewModule()); err != nil {
		t.Fatal(err)
	}
	if err := registry.SetStatus(ctx, cameras.ModuleID, "disabled", ""); err != nil {
		t.Fatal(err)
	}

	service := &fakeCamerasService{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	const nodeID = "00000000-0000-4000-8000-000000000000"
	handler := NewWithCameras(
		nodeID,
		logger,
		store,
		defaultFakeSecurity(),
		nil,
		nil,
		registry,
		nil,
		realtime.New(nodeID, logger),
		service,
	)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/modules/cameras/control",
		strings.NewReader(`{"operation":"enable"}`),
	)
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("enable response = %d: %s", rec.Code, rec.Body.String())
	}
	if service.refresh != 1 {
		t.Fatalf("refresh count = %d, want 1", service.refresh)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/modules/navigation", nil)
	req.Header.Set("Authorization", "Bearer test")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("navigation response = %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"/modules/cameras"`) {
		t.Fatalf("Cameras navigation missing: %s", rec.Body.String())
	}
}
