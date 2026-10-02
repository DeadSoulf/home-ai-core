package api

import (
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
	"github.com/DeadSoulf/home-ai-core/internal/state"
)

type apiNVRSecrets struct {
	values map[nvr.SecretRef]nvr.CameraCredential
}

func (s *apiNVRSecrets) PutCameraCredential(
	_ context.Context,
	_ string,
	credential nvr.CameraCredential,
) (nvr.SecretRef, error) {
	ref := nvr.SecretRef("sec_api_test")
	if s.values == nil {
		s.values = map[nvr.SecretRef]nvr.CameraCredential{}
	}
	s.values[ref] = credential
	return ref, nil
}

func (s *apiNVRSecrets) ResolveCameraCredential(
	_ context.Context,
	ref nvr.SecretRef,
) (nvr.CameraCredential, error) {
	return s.values[ref], nil
}

func (s *apiNVRSecrets) DeleteCameraCredential(_ context.Context, ref nvr.SecretRef) error {
	delete(s.values, ref)
	return nil
}

type apiNVRProber struct{}

func (apiNVRProber) Available() bool { return true }

func (apiNVRProber) Probe(_ context.Context, request nvr.ProbeRequest) (nvr.ProbeResult, error) {
	return nvr.ProbeResult{
		Codec:      "h264",
		Width:      1280,
		Height:     720,
		FPS:        20,
		BitrateBPS: 2_000_000,
		HasAudio:   true,
	}, nil
}

func TestNVRCameraOnboardingAPIKeepsCredentialsPrivate(t *testing.T) {
	ctx := context.Background()
	store, err := state.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if _, err := store.CreateOwner(
		ctx,
		"usr-test",
		"owner",
		"Owner",
		"test-password-hash",
		time.Now(),
	); err != nil {
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
	sec.actor.Permissions = append(sec.actor.Permissions, nvr.PermissionCameraManage, nvr.PermissionCameraList)

	service := nvr.NewServiceWithDependencies(store, &apiNVRSecrets{}, apiNVRProber{})
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

	body := `{
		"name":"Driveway",
		"address":"rtsp://192.0.2.44/stream",
		"username":"viewer",
		"password":"camera-secret",
		"transport":"tcp",
		"recording_mode":"off",
		"audio_enabled":true
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/nvr/cameras/test", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("camera test status = %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "camera-secret") || strings.Contains(rec.Body.String(), "viewer") {
		t.Fatalf("camera test leaked credentials: %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/nvr/cameras", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("camera create status = %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "camera-secret") || strings.Contains(rec.Body.String(), "sec_api_test") {
		t.Fatalf("camera create leaked secret: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"has_credentials":true`) {
		t.Fatalf("camera create did not report credential presence: %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/nvr/cameras", nil)
	req.Header.Set("Authorization", "Bearer test")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("camera list status = %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "192.0.2.44") || strings.Contains(rec.Body.String(), "camera-secret") {
		t.Fatalf("camera list leaked private configuration: %s", rec.Body.String())
	}

	cameras, err := store.ListNVRCameras(ctx)
	if err != nil || len(cameras) != 1 {
		t.Fatalf("stored cameras = %#v err=%v", cameras, err)
	}
	cameraID := cameras[0].ID
	req = httptest.NewRequest(http.MethodGet, "/api/v1/nvr/cameras/"+cameraID, nil)
	req.Header.Set("Authorization", "Bearer test")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("camera detail status = %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "rtsp://192.0.2.44/stream") {
		t.Fatalf("camera detail missing address: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "camera-secret") || strings.Contains(rec.Body.String(), "sec_api_test") {
		t.Fatalf("camera detail leaked secret: %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/v1/nvr/cameras/"+cameraID, nil)
	req.Header.Set("Authorization", "Bearer test")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("camera delete status = %d: %s", rec.Code, rec.Body.String())
	}
}


type apiONVIFDiscoverer struct{}

func (apiONVIFDiscoverer) Discover(context.Context) ([]nvr.ONVIFDevice, error) {
	return []nvr.ONVIFDevice{{
		ID:      "onvif_api_test",
		Name:    "Front Door",
		Address: "http://192.168.1.60/onvif/device_service",
		IP:      "192.168.1.60",
	}}, nil
}

type apiONVIFClient struct{}

func (apiONVIFClient) Profiles(
	_ context.Context,
	_ string,
	credential nvr.CameraCredential,
) ([]nvr.ONVIFProfile, error) {
	if credential.Username != "viewer" || credential.Password != "camera-secret" {
		return nil, nvr.ErrONVIFAuthentication
	}
	return []nvr.ONVIFProfile{
		{
			Token: "main", Name: "Main", StreamURI: "rtsp://192.168.1.60/main",
			Codec: "h264", Width: 1920, Height: 1080, FPS: 25, BitrateBPS: 4_000_000, HasAudio: true,
		},
		{
			Token: "sub", Name: "Sub", StreamURI: "rtsp://192.168.1.60/sub",
			Codec: "h264", Width: 640, Height: 360, FPS: 10, BitrateBPS: 500_000,
		},
	}, nil
}

func TestNVRONVIFDiscoveryProfilesAndImportAPI(t *testing.T) {
	ctx := context.Background()
	store, err := state.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if _, err := store.CreateOwner(
		ctx,
		"usr-onvif",
		"owner",
		"Owner",
		"test-password-hash",
		time.Now(),
	); err != nil {
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
	sec.actor.Permissions = append(sec.actor.Permissions, nvr.PermissionCameraManage, nvr.PermissionCameraList)
	service := nvr.NewServiceWithONVIFDependencies(
		store,
		&apiNVRSecrets{},
		apiNVRProber{},
		apiONVIFDiscoverer{},
		apiONVIFClient{},
	)
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

	req := httptest.NewRequest(http.MethodPost, "/api/v1/nvr/onvif/discover", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "192.168.1.60") {
		t.Fatalf("ONVIF discovery status = %d: %s", rec.Code, rec.Body.String())
	}

	profilesBody := `{
		"address":"http://192.168.1.60/onvif/device_service",
		"username":"viewer",
		"password":"camera-secret"
	}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/nvr/onvif/profiles", strings.NewReader(profilesBody))
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"token":"main"`) {
		t.Fatalf("ONVIF profiles status = %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "camera-secret") || strings.Contains(rec.Body.String(), "viewer") {
		t.Fatalf("ONVIF profiles leaked credentials: %s", rec.Body.String())
	}

	importBody := `{
		"name":"Front Door ONVIF",
		"address":"http://192.168.1.60/onvif/device_service",
		"username":"viewer",
		"password":"camera-secret",
		"main_profile_token":"main",
		"sub_profile_token":"sub",
		"transport":"tcp",
		"recording_mode":"off",
		"audio_enabled":true
	}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/nvr/onvif/import", strings.NewReader(importBody))
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("ONVIF import status = %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "camera-secret") || strings.Contains(rec.Body.String(), "sec_api_test") {
		t.Fatalf("ONVIF import leaked secret: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"source_type":"onvif"`) {
		t.Fatalf("ONVIF import did not preserve source type: %s", rec.Body.String())
	}

	cameras, err := store.ListNVRCameras(ctx)
	if err != nil || len(cameras) != 1 {
		t.Fatalf("stored cameras = %#v err=%v", cameras, err)
	}
	if cameras[0].Address != "rtsp://192.168.1.60/main" {
		t.Fatalf("stored runtime address = %q", cameras[0].Address)
	}
	meta, err := store.NVRONVIFSource(ctx, cameras[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if meta.MainProfileToken != "main" || meta.SubProfileToken != "sub" {
		t.Fatalf("stored ONVIF metadata = %#v", meta)
	}
}
