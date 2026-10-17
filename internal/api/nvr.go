package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/modules"
	"github.com/DeadSoulf/home-ai-core/internal/security"
)

const maxNVRJSONBytes = 2 << 20

var nvrSafeSegment = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9_.-]{0,119}$`)

type nvrCamera struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	SourceType string `json:"source_type"`
	IP         string `json:"ip,omitempty"`
	Port       int    `json:"port,omitempty"`
	Username   string `json:"username,omitempty"`
	Channel    int    `json:"channel,omitempty"`
	Enabled    bool   `json:"enabled"`
	Recording  bool   `json:"recording"`
}

type nvrStatusResponse struct {
	OK              bool    `json:"ok"`
	Service         string  `json:"service"`
	Cameras         int     `json:"cameras"`
	MaxCameras      int     `json:"max_cameras"`
	ActiveRecorders int     `json:"active_recorders"`
	Recordings      int     `json:"recordings"`
	StorageBytes    int64   `json:"storage_bytes"`
	RetentionDays   int     `json:"retention_days"`
	MaxStorageGB    float64 `json:"max_storage_gb"`
}

type nvrRecording struct {
	ID        int64  `json:"id"`
	CameraID  string `json:"camera_id"`
	StartedAt int64  `json:"started_at"`
	EndedAt   int64  `json:"ended_at"`
	SizeBytes int64  `json:"size_bytes"`
	PlayURL   string `json:"play_url,omitempty"`
}

// The endpoint comes exclusively from the UID-protected updater helper, and
// is a verified container IP rather than a caller-supplied URL. Docker access
// never flows through the public HTTP handler.
func (s *server) nvrAddress(ctx context.Context) (string, error) {
	resolve := s.nvrEndpoint
	if resolve == nil {
		resolve = modules.ResolveNVREndpoint
	}
	address, err := resolve(ctx)
	if err != nil {
		return "", err
	}
	ip, err := netip.ParseAddr(address)
	if err != nil || !ip.Is4() || !ip.IsPrivate() || ip.IsLoopback() {
		return "", errors.New("invalid private NVR address")
	}
	return address, nil
}

func (s *server) nvrReady(w http.ResponseWriter, r *http.Request) bool {
	if s.modules == nil {
		writeAPIError(w, r, http.StatusServiceUnavailable, "nvr_not_running", "NVR module service is unavailable", nil)
		return false
	}
	item, err := s.modules.Get(r.Context(), "nvr.module")
	if err != nil || item.Status != "enabled" {
		writeAPIError(w, r, http.StatusServiceUnavailable, "nvr_not_running",
			"NVR module is not installed or enabled", nil)
		return false
	}
	return true
}

// No connection pooling, redirect-following, environmental proxy, forwarded
// cookies, Authorization header, or client-selected destination.
func (s *server) nvrRequest(ctx context.Context, method, endpoint string, payload []byte) (*http.Response, error) {
	address, err := s.nvrAddress(ctx)
	if err != nil {
		return nil, err
	}
	target := "http://" + net.JoinHostPort(address, "8090") + endpoint
	var body io.Reader
	if method == http.MethodPost {
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, err
	}
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	transport := &http.Transport{
		Proxy: nil, DisableKeepAlives: true,
		DialContext:           (&net.Dialer{Timeout: 4 * time.Second}).DialContext,
		ResponseHeaderTimeout: 12 * time.Second,
	}
	defer transport.CloseIdleConnections()
	var roundTrip http.RoundTripper = transport
	if s.nvrTransport != nil {
		roundTrip = s.nvrTransport
	}
	client := &http.Client{
		Transport:     roundTrip,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return client.Do(req)
}

func (s *server) nvrJSON(w http.ResponseWriter, r *http.Request, method, endpoint string, payload []byte, result any) bool {
	ctx, cancel := context.WithTimeout(r.Context(), 19*time.Second)
	defer cancel()
	response, err := s.nvrRequest(ctx, method, endpoint, payload)
	if err != nil {
		writeAPIError(w, r, http.StatusBadGateway, "nvr_unavailable", "Unable to connect to the NVR module", nil)
		return false
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		writeAPIError(w, r, http.StatusBadGateway, "nvr_upstream_error", "The NVR module rejected the request", nil)
		return false
	}
	if !strings.HasPrefix(response.Header.Get("Content-Type"), "application/json") {
		writeAPIError(w, r, http.StatusBadGateway, "nvr_upstream_error", "Unexpected NVR response type", nil)
		return false
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxNVRJSONBytes+1))
	if err != nil || len(body) > maxNVRJSONBytes || json.Unmarshal(body, result) != nil {
		writeAPIError(w, r, http.StatusBadGateway, "nvr_upstream_error", "Invalid NVR JSON response", nil)
		return false
	}
	return true
}

func (s *server) nvrStatus(w http.ResponseWriter, r *http.Request, _ security.Actor, _ authSource) {
	if !s.nvrReady(w, r) {
		return
	}
	var result nvrStatusResponse
	if !s.nvrJSON(w, r, http.MethodGet, "/api/nvr/status", nil, &result) {
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *server) nvrCameras(w http.ResponseWriter, r *http.Request, _ security.Actor, _ authSource) {
	if !s.nvrReady(w, r) {
		return
	}
	// Typed decoding prevents the upstream's rtsp_url (which may contain
	// credentials) and other module-specific secrets from reaching the browser.
	var result struct {
		OK      bool        `json:"ok"`
		Cameras []nvrCamera `json:"cameras"`
	}
	if !s.nvrJSON(w, r, http.MethodGet, "/api/nvr/cameras", nil, &result) {
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func validNVROutputPath(value string) bool {
	parts := strings.Split(value, "/")
	if len(parts) != 3 || (parts[0] != "recordings" && parts[0] != "hls") {
		return false
	}
	for _, part := range parts[1:] {
		if part == "." || part == ".." || !nvrSafeSegment.MatchString(part) {
			return false
		}
	}
	if parts[0] == "recordings" {
		return strings.HasSuffix(parts[2], ".mp4")
	}
	return strings.HasSuffix(parts[2], ".m3u8") || strings.HasSuffix(parts[2], ".ts")
}

func (s *server) nvrRecordings(w http.ResponseWriter, r *http.Request, _ security.Actor, _ authSource) {
	if !s.nvrReady(w, r) {
		return
	}
	params := url.Values{"limit": {"200"}}
	if camera := r.URL.Query().Get("camera_id"); camera != "" {
		if !nvrSafeSegment.MatchString(camera) || camera == ".." {
			writeAPIError(w, r, http.StatusBadRequest, "invalid_camera_id", "Invalid camera ID", nil)
			return
		}
		params.Set("camera_id", camera)
	}
	var upstream struct {
		OK         bool `json:"ok"`
		Recordings []struct {
			ID        int64  `json:"id"`
			CameraID  string `json:"camera_id"`
			StartedAt int64  `json:"started_at"`
			EndedAt   int64  `json:"ended_at"`
			SizeBytes int64  `json:"size_bytes"`
			Path      string `json:"path"`
		} `json:"recordings"`
	}
	if !s.nvrJSON(w, r, http.MethodGet, "/api/nvr/recordings?"+params.Encode(), nil, &upstream) {
		return
	}
	rows := make([]nvrRecording, 0, len(upstream.Recordings))
	for _, source := range upstream.Recordings {
		item := nvrRecording{
			ID: source.ID, CameraID: source.CameraID,
			StartedAt: source.StartedAt, EndedAt: source.EndedAt,
			SizeBytes: source.SizeBytes,
		}
		if validNVROutputPath(source.Path) && strings.HasPrefix(source.Path, "recordings/") {
			item.PlayURL = "/api/v1/nvr/output/" + source.Path
		}
		rows = append(rows, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": upstream.OK, "recordings": rows})
}

func (s *server) nvrAddCamera(w http.ResponseWriter, r *http.Request, actor security.Actor, source authSource) {
	if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}
	if !s.nvrReady(w, r) {
		return
	}
	var input struct {
		Name       string `json:"name"`
		SourceType string `json:"source_type"`
		IP         string `json:"ip"`
		Port       int    `json:"port"`
		Username   string `json:"username"`
		Password   string `json:"password"`
		Channel    int    `json:"channel"`
		RTSPURL    string `json:"rtsp_url"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_camera", "Invalid camera configuration", nil)
		return
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_camera", "Camera configuration must contain exactly one JSON object", nil)
		return
	}
	if len(input.Name) == 0 || len(input.Name) > 128 ||
		(input.SourceType != "rtsp" && input.SourceType != "hikvision") {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_camera", "Name and supported source type are required", nil)
		return
	}
	if input.SourceType == "rtsp" && !strings.HasPrefix(strings.ToLower(input.RTSPURL), "rtsp://") &&
		!strings.HasPrefix(strings.ToLower(input.RTSPURL), "rtsps://") {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_camera", "Invalid RTSP address", nil)
		return
	}
	if input.SourceType == "hikvision" && (input.IP == "" || input.Username == "" || input.Password == "") {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_camera", "Hikvision address and credentials are required", nil)
		return
	}
	if input.Port < 0 || input.Port > 65535 || input.Channel < 0 || input.Channel > 1024 {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_camera", "Invalid port or channel", nil)
		return
	}
	data, err := json.Marshal(input)
	if err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_camera", "Invalid camera", nil)
		return
	}
	var ignored json.RawMessage
	if !s.nvrJSON(w, r, http.MethodPost, "/api/nvr/cameras", data, &ignored) {
		return
	}
	s.security.RecordAudit(r.Context(), s.securityRequestContext(r), actor,
		"nvr.camera.create", "module", "nvr.module", "success",
		map[string]any{"source_type": input.SourceType})
	// Never echo a credential-containing RTSP URL back to the browser.
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true})
}

func (s *server) nvrControlCamera(w http.ResponseWriter, r *http.Request, actor security.Actor, source authSource) {
	if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}
	if !s.nvrReady(w, r) {
		return
	}
	id, action := r.PathValue("cameraID"), r.PathValue("action")
	if !nvrSafeSegment.MatchString(id) || id == ".." ||
		(action != "start" && action != "stop" && action != "restart") {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_camera_operation", "Unsupported camera operation", nil)
		return
	}
	var ignored json.RawMessage
	if !s.nvrJSON(w, r, http.MethodPost, "/api/nvr/cameras/"+id+"/"+action, []byte("{}"), &ignored) {
		return
	}
	s.security.RecordAudit(r.Context(), s.securityRequestContext(r), actor,
		"nvr.camera."+action, "module", "nvr.module", "success",
		map[string]any{"camera_id": id})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// Only the module ID from the authenticated route is used as an upstream
// path segment. The NVR resolves the configured camera credentials internally.
func (s *server) nvrLive(w http.ResponseWriter, r *http.Request, actor security.Actor, source authSource) {
	if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}
	if !s.nvrReady(w, r) { return }
	id, action := r.PathValue("cameraID"), r.PathValue("action")
	if !nvrSafeSegment.MatchString(id) || id == ".." || (action != "start" && action != "stop") {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_camera_operation", "Unsupported live preview operation", nil)
		return
	}
	var upstream struct {
		OK bool `json:"ok"`
		Key string `json:"key"`
	}
	if !s.nvrJSON(w, r, http.MethodPost, "/api/nvr/live/"+id+"/"+action, []byte("{}"), &upstream) {
		return
	}
	result := map[string]any{"ok": upstream.OK}
	if action == "start" && upstream.OK {
		// The stream key is derived locally, not trusted from the upstream.
		// This avoids arbitrary file URLs or escapes in the response.
		result["playlist_url"] = "/api/v1/nvr/output/hls/camera_"+id+"/index.m3u8"
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *server) nvrDeleteCamera(w http.ResponseWriter, r *http.Request, actor security.Actor, source authSource) {
	if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}
	if !s.nvrReady(w, r) { return }
	id := r.PathValue("cameraID")
	if !nvrSafeSegment.MatchString(id) || id == ".." {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_camera_id", "Invalid camera ID", nil)
		return
	}
	// Retain the recording files and index. A separate, deliberately
	// destructive workflow must be implemented before deletion of archives.
	var upstream struct {
		OK bool `json:"ok"`
		Deleted bool `json:"deleted"`
	}
	if !s.nvrJSON(w, r, http.MethodDelete, "/api/nvr/cameras/"+id, nil, &upstream) {
		return
	}
	if !upstream.Deleted {
		writeAPIError(w, r, http.StatusNotFound, "nvr_camera_not_found", "Camera not found", nil)
		return
	}
	s.security.RecordAudit(r.Context(), s.securityRequestContext(r), actor, "nvr.camera.remove", "nvr.camera", id, "success",
		map[string]any{"recordings_preserved": true})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "recordings_preserved": true})
}

func (s *server) nvrOutput(w http.ResponseWriter, r *http.Request, _ security.Actor, _ authSource) {
	if !s.nvrReady(w, r) {
		return
	}
	path := r.PathValue("path")
	if !validNVROutputPath(path) {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_nvr_media", "Invalid NVR media path", nil)
		return
	}
	// Archived MP4 files are persisted under the Core-owned module data root.
	// Serve them from a rooted filesystem with native HTTP Range support:
	// NVR's original file endpoint loads the entire video into memory and
	// does not implement Range requests, which is unsafe for large recordings.
	if strings.HasPrefix(path, "recordings/") {
		s.nvrRecordingFile(w, r, path)
		return
	}
	response, err := s.nvrRequest(r.Context(), http.MethodGet, "/output/"+path, nil)
	if err != nil {
		writeAPIError(w, r, http.StatusBadGateway, "nvr_unavailable", "NVR media is unavailable", nil)
		return
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		writeAPIError(w, r, http.StatusBadGateway, "nvr_media_unavailable", "NVR media could not be read", nil)
		return
	}
	contentType := "video/mp4"
	if strings.HasSuffix(path, ".m3u8") {
		contentType = "application/vnd.apple.mpegurl"
	}
	if strings.HasSuffix(path, ".ts") {
		contentType = "video/mp2t"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Disposition", "inline")
	if count, err := strconv.ParseInt(response.Header.Get("Content-Length"), 10, 64); err == nil && count >= 0 {
		w.Header().Set("Content-Length", fmt.Sprint(count))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, response.Body)
}

func (s *server) nvrRecordingFile(w http.ResponseWriter, r *http.Request, relativePath string) {
	dataRoot := s.nvrDataRoot
	if dataRoot == "" {
		dataRoot = "/var/lib/home-ai-modules/nvr.module"
	}
	root, err := os.OpenRoot(dataRoot)
	if err != nil {
		writeAPIError(w, r, http.StatusServiceUnavailable, "nvr_media_unavailable", "NVR recording storage is unavailable", nil)
		return
	}
	defer root.Close()
	file, err := root.Open(relativePath)
	if err != nil {
		writeAPIError(w, r, http.StatusNotFound, "nvr_media_unavailable", "Recording not found", nil)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		writeAPIError(w, r, http.StatusNotFound, "nvr_media_unavailable", "Recording not found", nil)
		return
	}
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Disposition", "inline")
	http.ServeContent(w, r, filepath.Base(relativePath), info.ModTime(), file)
}
