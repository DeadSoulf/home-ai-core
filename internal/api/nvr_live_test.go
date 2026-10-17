package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DeadSoulf/home-ai-core/internal/security"
)

func TestNVRLivePlaylistUsesOnlyAuthenticatedCoreMedia(t *testing.T) {
	s := nvrTestUpstream(t, `{"ok":true,"playlist_url":"/output/hls/saved_cam01/index.m3u8"}`)
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
	s = nvrTestUpstream(t, `{"ok":true,"playlist_url":"http://evil.example/output/hls/camera/index.m3u8"}`)
	response = httptest.NewRecorder()
	s.nvrLiveCamera(response, request, security.Actor{}, authBearer)
	if response.Code != http.StatusBadGateway {
		t.Fatalf("untrusted upstream video URL accepted: %d", response.Code)
	}
}

func TestNVRDeleteRequiresSeparateArchiveConfirmation(t *testing.T) {
	s := nvrTestUpstream(t, `{"ok":true,"deleted":true}`)
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
