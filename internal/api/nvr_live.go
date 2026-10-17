package api

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/DeadSoulf/home-ai-core/internal/security"
)

// Live-video API never receives an RTSP URL or camera password from the UI.
// The NVR itself retrieves the stored credentials, and Core permits only a
// whitelisted HLS output path back to authenticated browsers.
func (s *server) nvrLiveCamera(w http.ResponseWriter, r *http.Request, actor security.Actor, source authSource) {
	if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}
	if !s.nvrReady(w, r) {
		return
	}
	cameraID := r.PathValue("cameraID")
	action := r.PathValue("action")
	if !nvrSafeSegment.MatchString(cameraID) || cameraID == ".." || cameraID == "." ||
		(action != "start" && action != "stop") {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_camera_operation", "Invalid live preview operation", nil)
		return
	}
	var response struct {
		OK          bool   `json:"ok"`
		Stopped     bool   `json:"stopped"`
		PlaylistURL string `json:"playlist_url"`
	}
	upstream := "/api/nvr/cameras/" + cameraID + "/live/" + action
	if !s.nvrJSON(w, r, http.MethodPost, upstream, []byte("{}"), &response) {
		return
	}
	if !response.OK {
		writeAPIError(w, r, http.StatusBadGateway, "nvr_live_failed", "Camera live preview failed", nil)
		return
	}
	if action == "stop" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "stopped": response.Stopped})
		return
	}
	rawURL, err := url.Parse(response.PlaylistURL)
	if err != nil || rawURL.IsAbs() || rawURL.Host != "" || rawURL.RawQuery != "" ||
		!strings.HasPrefix(rawURL.Path, "/output/hls/") {
		writeAPIError(w, r, http.StatusBadGateway, "invalid_nvr_stream", "Unexpected camera playlist path", nil)
		return
	}
	relative := strings.TrimPrefix(rawURL.Path, "/output/")
	if !validNVROutputPath(relative) || !strings.HasSuffix(relative, "/index.m3u8") {
		writeAPIError(w, r, http.StatusBadGateway, "invalid_nvr_stream", "Unexpected camera playlist path", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
		"playlist_url": "/api/v1/nvr/output/" + relative,
	})
}

func (s *server) nvrDeleteCamera(w http.ResponseWriter, r *http.Request, actor security.Actor, source authSource) {
	if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}
	if !s.nvrReady(w, r) {
		return
	}
	cameraID := r.PathValue("cameraID")
	if !nvrSafeSegment.MatchString(cameraID) || cameraID == ".." || cameraID == "." {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_camera_id", "Invalid camera ID", nil)
		return
	}
	deleteRecordings := r.URL.Query().Get("delete_recordings") == "true"
	if deleteRecordings && r.Header.Get("X-NVR-Delete-Recordings") != "confirm" {
		writeAPIError(w, r, http.StatusBadRequest, "nvr_delete_confirmation_required",
			"Explicit archive deletion confirmation is required", nil)
		return
	}
	endpoint := "/api/nvr/cameras/" + cameraID
	if deleteRecordings {
		endpoint += "?delete_recordings=1"
	}
	var result struct {
		OK      bool `json:"ok"`
		Deleted bool `json:"deleted"`
	}
	if !s.nvrJSON(w, r, http.MethodDelete, endpoint, nil, &result) {
		return
	}
	if !result.OK {
		writeAPIError(w, r, http.StatusBadGateway, "nvr_delete_failed", "Unable to remove camera", nil)
		return
	}
	if result.Deleted {
		s.security.RecordAudit(r.Context(), s.securityRequestContext(r), actor,
			"nvr.camera.delete", "module", "nvr.module", "success",
			map[string]any{"camera_id": cameraID, "delete_recordings": deleteRecordings})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": result.Deleted})
}
