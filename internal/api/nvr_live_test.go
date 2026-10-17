package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DeadSoulf/home-ai-core/internal/modules"
	"github.com/DeadSoulf/home-ai-core/internal/security"
)

func TestNVRLivePlaylistUsesOnlyAuthenticatedCoreMedia(t *testing.T) {
	s := liveNVRTestUpstream(t, `{"ok":true,"playlist_url":"/output/hls/saved_cam01/index.m3u8"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/nvr/cameras/cam01/live/start",
		strings.NewReader("{}"))
	request.SetPathValue("cameraID", "cam01")
	request.SetPathValue("action", "start")
	response := httptest.NewRecorder()
	s.nvrLiveCamera(response, request, security.Actor{}, authBearer)
	if response.Code != http.StatusOK ||
		!strings.Contains(response.Body.String(), "/api/v1/nvr/output/hls/saved_cam01/index.m3u8") {
		t.Fatalf("HLS stream is not scoped by Core: %d %s", response.Code, response.Body.String())
	}
	s = liveNVRTestUpstream(t, `{"ok":true,"playlist_url":"http://evil.example/output/hls/camera/index.m3u8"}`)
	response = httptest.NewRecorder()
	s.nvrLiveCamera(response, request, security.Actor{}, authBearer)
	if response.Code != http.StatusBadGateway {
		t.Fatalf("untrusted upstream video URL accepted: %d", response.Code)
	}
}

func TestNVRDeleteRequiresSeparateArchiveConfirmation(t *testing.T) {
	s := liveNVRTestUpstream(t, `{"ok":true,"deleted":true}`)
	s.security = defaultFakeSecurity()
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/nvr/cameras/cam1?delete_recordings=true", nil)
	request.SetPathValue("cameraID", "cam1")
	response := httptest.NewRecorder()
	s.nvrDeleteCamera(response, request, security.Actor{}, authBearer)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("deleted archive without confirmation: %d", response.Code)
	}
	request.Header.Set("X-NVR-Delete-Recordings", "confirm")
	response = httptest.NewRecorder()
	s.nvrDeleteCamera(response, request, security.Actor{}, authBearer)
	if response.Code != http.StatusOK {
		t.Fatalf("confirmed deletion failed: %d %s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodDelete, "/api/v1/nvr/cameras/cam1", nil)
	request.SetPathValue("cameraID", "cam1")
	response = httptest.NewRecorder()
	s.nvrDeleteCamera(response, request, security.Actor{}, authBearer)
	if response.Code != http.StatusOK {
		t.Fatalf("safe delete failed: %d", response.Code)
	}
}

func TestNVRLiveAndDeleteRejectMissingCookieCSRF(t *testing.T) {
	s := &server{modules: nvrTestService(true)}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/nvr/cameras/cam1/live/start", nil)
	request.SetPathValue("cameraID", "cam1")
	request.SetPathValue("action", "start")
	response := httptest.NewRecorder()
	s.nvrLiveCamera(response, request, security.Actor{}, authCookie)
	if response.Code != http.StatusForbidden {
		t.Fatalf("live mutation without CSRF: %d", response.Code)
	}
	request = httptest.NewRequest(http.MethodDelete, "/api/v1/nvr/cameras/cam1", nil)
	request.SetPathValue("cameraID", "cam1")
	response = httptest.NewRecorder()
	s.nvrDeleteCamera(response, request, security.Actor{}, authCookie)
	if response.Code != http.StatusForbidden {
		t.Fatalf("camera delete without CSRF: %d", response.Code)
	}
}

func liveNVRTestUpstream(t *testing.T, body string) *server {
	s := nvrTestUpstream(t, body)
	s.modules = fakeModules{items: []modules.Registered{{
		Manifest: modules.Manifest{ID: "nvr.module", Name: "NVR", Version: "1.1.3"},
		Status:   "enabled",
	}}}
	return s
}

func TestNVROldRuntimeCannotDeleteArchiveIndexes(t *testing.T) {
	// The NVR 1.1.2 image did not support soft delete and would cascade
	// records out of SQLite. Core must refuse both DELETE and managed live
	// operations until the runtime has been upgraded.
	s := nvrTestUpstream(t, `{"ok":true,"deleted":true}`)
	s.security = defaultFakeSecurity()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/nvr/cameras/old-camera", nil)
	req.SetPathValue("cameraID", "old-camera")
	result := httptest.NewRecorder()
	s.nvrDeleteCamera(result, req, security.Actor{}, authBearer)
	if result.Code != http.StatusConflict || !strings.Contains(result.Body.String(), "nvr_upgrade_required") {
		t.Fatalf("old NVR runtime accepted unsafe deletion: %d %s", result.Code, result.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/nvr/cameras/old-camera/live/start", nil)
	req.SetPathValue("cameraID", "old-camera")
	req.SetPathValue("action", "start")
	result = httptest.NewRecorder()
	s.nvrLiveCamera(result, req, security.Actor{}, authBearer)
	if result.Code != http.StatusConflict {
		t.Fatalf("old NVR runtime accepted new live endpoint: %d", result.Code)
	}
}
