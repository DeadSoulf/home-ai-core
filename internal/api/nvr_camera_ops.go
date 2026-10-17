package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/DeadSoulf/home-ai-core/internal/security"
)

// The legacy NVR image's DELETE endpoint dropped recording index rows even
// when it kept MP4 files. Managed live viewing and safe camera deletion are
// available only after the server-side NVR implementation has been upgraded.
func (s *server) nvrRequiresCurrentRuntime(w http.ResponseWriter, r *http.Request) bool {
	item, err := s.modules.Get(r.Context(), "nvr.module")
	if err != nil {
		writeAPIError(w, r, http.StatusServiceUnavailable, "nvr_not_running", "NVR is unavailable", nil)
		return false
	}
	version := strings.SplitN(item.Manifest.Version, "-", 2)[0]
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		writeAPIError(w, r, http.StatusConflict, "nvr_upgrade_required", "Upgrade NVR to use managed live video and camera deletion", nil)
		return false
	}
	major, e1 := strconv.Atoi(parts[0])
	minor, e2 := strconv.Atoi(parts[1])
	patch, e3 := strconv.Atoi(parts[2])
	if e1 != nil || e2 != nil || e3 != nil || major < 1 ||
		(major == 1 && minor < 1) ||
		(major == 1 && minor == 1 && patch < 3) {
		writeAPIError(w, r, http.StatusConflict, "nvr_upgrade_required",
			"Upgrade NVR to version 1.1.3 or later before changing camera settings", nil)
		return false
	}
	return true
}

func validManagedCameraID(id string) bool {
	return id != "." && id != ".." && nvrSafeSegment.MatchString(id)
}

func (s *server) nvrDeleteCamera(w http.ResponseWriter, r *http.Request, actor security.Actor, source authSource) {
	if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}
	if !s.nvrReady(w, r) || !s.nvrRequiresCurrentRuntime(w, r) {
		return
	}
	id := r.PathValue("cameraID")
	if !validManagedCameraID(id) {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_camera_id", "Invalid camera ID", nil)
		return
	}
	var input struct {
		Confirm          string `json:"confirm"`
		DeleteRecordings bool   `json:"delete_recordings"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || input.Confirm != id {
		writeAPIError(w, r, http.StatusBadRequest, "nvr_delete_confirmation_required",
			"Confirm the exact camera ID to delete it", nil)
		return
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_camera_delete",
			"Delete request must contain one JSON object", nil)
		return
	}
	endpoint := "/api/nvr/cameras/" + url.PathEscape(id)
	if input.DeleteRecordings {
		endpoint += "?delete_recordings=1"
	}
	var deleted struct {
		OK      bool `json:"ok"`
		Deleted bool `json:"deleted"`
	}
	if !s.nvrJSON(w, r, http.MethodDelete, endpoint, nil, &deleted) {
		return
	}
	if !deleted.OK || !deleted.Deleted {
		writeAPIError(w, r, http.StatusNotFound, "nvr_camera_not_found", "Camera not found", nil)
		return
	}
	s.security.RecordAudit(r.Context(), s.securityRequestContext(r), actor,
		"nvr.camera.delete", "module", "nvr.module", "success",
		map[string]any{"camera_id": id, "delete_recordings": input.DeleteRecordings})
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "deleted": true, "recordings_preserved": !input.DeleteRecordings,
	})
}

func (s *server) nvrCameraLive(w http.ResponseWriter, r *http.Request, actor security.Actor, source authSource) {
	if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}
	if !s.nvrReady(w, r) || !s.nvrRequiresCurrentRuntime(w, r) {
		return
	}
	id, action := r.PathValue("cameraID"), r.PathValue("action")
	if !validManagedCameraID(id) || (action != "start" && action != "stop") {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_camera_live",
			"Invalid camera or video operation", nil)
		return
	}
	var input struct {
		StreamType int `json:"stream_type"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || input.StreamType < 0 || input.StreamType > 2 {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_live_stream",
			"Choose a valid video stream type", nil)
		return
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_live_stream",
			"Request must contain one JSON object", nil)
		return
	}
	raw, _ := json.Marshal(input)
	upstream := "/api/nvr/cameras/" + url.PathEscape(id) + "/live/" + action
	var response struct {
		OK      bool `json:"ok"`
		Stopped bool `json:"stopped"`
	}
	if !s.nvrJSON(w, r, http.MethodPost, upstream, raw, &response) {
		return
	}
	if !response.OK {
		writeAPIError(w, r, http.StatusBadGateway, "nvr_live_unavailable",
			"NVR video stream is unavailable", nil)
		return
	}
	result := map[string]any{"ok": true}
	if action == "start" {
		// Never trust a module-supplied URL or pass its credentials to the
		// browser. HLS chunks pass through the Core-authenticated media path.
		result["playlist_url"] = fmt.Sprintf("/api/v1/nvr/output/hls/camera_%s_s%d/index.m3u8", id, input.StreamType)
	} else {
		result["stopped"] = response.Stopped
	}
	s.security.RecordAudit(r.Context(), s.securityRequestContext(r), actor,
		"nvr.live."+action, "module", "nvr.module", "success",
		map[string]any{"camera_id": id, "stream_type": input.StreamType})
	writeJSON(w, http.StatusOK, result)
}
