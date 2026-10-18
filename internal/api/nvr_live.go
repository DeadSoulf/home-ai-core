package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/DeadSoulf/home-ai-core/internal/security"
)

// Legacy NVR 1.1.2 deletes foreign-keyed archive indexes when removing a
// camera. Requiring 1.1.3+ prevents data loss on existing installations.
func nvrSupportsSavedCameraActions(version string) bool {
	parts := strings.Split(strings.TrimPrefix(version, "v"), ".")
	if len(parts) != 3 {
		return false
	}
	numbers := [3]int{}
	for index, part := range parts {
		if index == 2 {
			part, _, _ = strings.Cut(part, "-")
		}
		value, err := strconv.Atoi(part)
		if err != nil || value < 0 {
			return false
		}
		numbers[index] = value
	}
	return numbers[0] > 1 || numbers[0] == 1 &&
		(numbers[1] > 1 || numbers[1] == 1 && numbers[2] >= 3)
}

func (s *server) nvrModern(w http.ResponseWriter, r *http.Request) bool {
	if !s.nvrReady(w, r) {
		return false
	}
	item, err := s.modules.Get(r.Context(), "nvr.module")
	if err != nil || !nvrSupportsSavedCameraActions(item.Manifest.Version) {
		writeAPIError(w, r, http.StatusConflict, "nvr_upgrade_required",
			"Update the NVR module to 1.1.3 or later to safely manage saved-camera live view and removal", nil)
		return false
	}
	return true
}

func validNVRCameraActionID(value string) bool {
	return value != "" && value != "." && value != ".." && nvrSafeSegment.MatchString(value)
}

func (s *server) nvrSavedLive(w http.ResponseWriter, r *http.Request, actor security.Actor, source authSource) {
	if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}
	if !s.nvrModern(w, r) {
		return
	}
	id, action := r.PathValue("cameraID"), r.PathValue("action")
	if !validNVRCameraActionID(id) || (action != "start" && action != "stop") {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_nvr_live_operation", "Invalid saved camera live request", nil)
		return
	}
	var result struct {
		OK          bool   `json:"ok"`
		Key         string `json:"key"`
		PlaylistURL string `json:"playlist_url"`
		Stopped     bool   `json:"stopped"`
	}
	if !s.nvrJSON(w, r, http.MethodPost, "/api/nvr/cameras/"+id+"/live/"+action, []byte("{}"), &result) {
		return
	}
	if !result.OK {
		writeAPIError(w, r, http.StatusBadGateway, "nvr_live_failed", "NVR could not manage live view", nil)
		return
	}
	if action == "start" {
		expectedKey := "saved_" + id
		// Don't pass arbitrary URLs or query strings from a camera's service
		// to the browser. The Core-authenticated media route is fixed.
		if result.Key != expectedKey || !validNVROutputPath("hls/"+expectedKey+"/index.m3u8") ||
			!strings.HasPrefix(result.PlaylistURL, "/output/hls/"+expectedKey+"/index.m3u8") {
			writeAPIError(w, r, http.StatusBadGateway, "nvr_live_invalid_response", "NVR returned an invalid live session", nil)
			return
		}
		s.security.RecordAudit(r.Context(), s.securityRequestContext(r), actor,
			"nvr.live.start", "module", "nvr.module", "success", map[string]any{"camera_id": id})
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": true, "playlist_url": "/api/v1/nvr/output/hls/" + expectedKey + "/index.m3u8",
		})
		return
	}
	s.security.RecordAudit(r.Context(), s.securityRequestContext(r), actor,
		"nvr.live.stop", "module", "nvr.module", "success", map[string]any{"camera_id": id})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "stopped": result.Stopped})
}

func (s *server) nvrDeleteCamera(w http.ResponseWriter, r *http.Request, actor security.Actor, source authSource) {
	if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}
	if !s.nvrModern(w, r) {
		return
	}
	id := r.PathValue("cameraID")
	if !validNVRCameraActionID(id) {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_camera_id", "Invalid camera ID", nil)
		return
	}
	var result struct {
		OK      bool `json:"ok"`
		Deleted bool `json:"deleted"`
	}
	if !s.nvrJSON(w, r, http.MethodDelete, "/api/nvr/cameras/"+id, nil, &result) {
		return
	}
	if !result.OK || !result.Deleted {
		writeAPIError(w, r, http.StatusNotFound, "nvr_camera_not_found", "Camera was not found or already removed", nil)
		return
	}
	// No delete_recordings parameter: NVR 1.1.3+ soft-deletes the camera and
	// retains both its files and recording index.
	s.security.RecordAudit(r.Context(), s.securityRequestContext(r), actor,
		"nvr.camera.remove", "module", "nvr.module", "success",
		map[string]any{"camera_id": id, "recordings_preserved": true})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "recordings_preserved": true})
}
