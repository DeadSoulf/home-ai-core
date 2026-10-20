package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/frigate"
	"github.com/DeadSoulf/home-ai-core/internal/modules"
	"github.com/DeadSoulf/home-ai-core/internal/security"
)

var frigateResourceID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

// The native HomeAiCore UI uses an allowlisted API bridge rather than executing
// module-supplied JavaScript in Core's authenticated origin. The internal Frigate
// port is admin-equivalent, so EVERY route is restricted to modules.manage.
func frigateGatewayPath(r *http.Request) (string, string, bool) {
	subpath := strings.TrimPrefix(r.URL.Path, "/api/v1/frigate/api/")
	parts := strings.Split(subpath, "/")
	if r.Method == http.MethodGet {
		switch subpath {
		case "version", "stats", "config", "config/raw":
			return "/api/" + subpath, "json", true
		case "events":
			values := url.Values{}
			limit := 50
			if value := r.URL.Query().Get("limit"); value != "" {
				parsed, err := strconv.Atoi(value)
				if err != nil || parsed < 1 || parsed > 200 {
					return "", "", false
				}
				limit = parsed
			}
			values.Set("limit", strconv.Itoa(limit))
			values.Set("include_thumbnails", "0")
			for _, key := range []string{"camera", "label"} {
				if value := r.URL.Query().Get(key); value != "" {
					if !frigateResourceID.MatchString(value) {
						return "", "", false
					}
					values.Set(key, value)
				}
			}
			return "/api/events?" + values.Encode(), "json", true
		}
		if len(parts) == 2 && parts[0] == "snapshot" && frigateResourceID.MatchString(parts[1]) && !strings.Contains(parts[1], "..") {
			return "/api/" + parts[1] + "/latest.jpg", "image", true
		}
		if len(parts) == 3 && parts[0] == "events" && frigateResourceID.MatchString(parts[1]) && !strings.Contains(parts[1], "..") {
			switch parts[2] {
			case "snapshot.jpg", "thumbnail.jpg":
				return "/api/events/" + parts[1] + "/" + parts[2], "image", true
			case "clip.mp4":
				return "/api/events/" + parts[1] + "/clip.mp4", "video", true
			}
		}
	}
	if r.Method == http.MethodPost && subpath == "config/save" {
		return "/api/config/save?save_option=saveonly", "json", true
	}
	return "", "", false
}
func (s *server) frigateGateway(w http.ResponseWriter, r *http.Request, actor security.Actor, source authSource) {
	path, kind, allowed := frigateGatewayPath(r)
	if !allowed {
		s.notFound(w, r)
		return
	}
	mutation := r.Method != http.MethodGet
	if mutation && source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}
	if mutation {
		if !s.moduleSubmitMu.TryLock() {
			writeModuleSubmitError(w, r, errModuleOperationInProgress)
			return
		}
		defer s.moduleSubmitMu.Unlock()
		active, err := s.activeModuleJobs(r.Context())
		if err != nil {
			writeAPIError(w, r, 503, "jobs_unavailable", "Cannot verify module operation state", nil)
			return
		}
		if len(active) > 0 {
			writeModuleSubmitError(w, r, errModuleOperationInProgress)
			return
		}
	}
	if s.modules == nil {
		writeAPIError(w, r, 503, "frigate_not_running", "Frigate is unavailable", nil)
		return
	}
	installed, err := s.modules.Get(r.Context(), frigate.ModuleID)
	if err != nil || installed.Status != "enabled" {
		writeAPIError(w, r, 503, "frigate_not_running", "Frigate is not installed or enabled", nil)
		return
	}
	var body []byte
	if mutation {
		body, err = io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil || len(bytes.TrimSpace(body)) == 0 {
			writeAPIError(w, r, 400, "invalid_frigate_config", "Configuration must be nonempty and at most 1 MiB", nil)
			return
		}
	}
	budget := 20 * time.Second
	if kind == "video" {
		budget = 10 * time.Minute
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(budget))
	}
	ctx, cancel := context.WithTimeout(r.Context(), budget)
	defer cancel()
	response, err := s.frigateRequest(ctx, r.Method, path, body, r.Header.Get("Range"))
	if err != nil {
		writeAPIError(w, r, 502, "frigate_unavailable", "Unable to contact the managed Frigate container", nil)
		return
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		status := http.StatusBadGateway
		if response.StatusCode == 404 {
			status = 404
		}
		if response.StatusCode == 400 {
			status = 400
		}
		message := "Frigate rejected the request"
		if mutation && response.StatusCode == 400 {
			data, _ := io.ReadAll(io.LimitReader(response.Body, 8192))
			var reply struct {
				Message string `json:"message"`
			}
			if json.Unmarshal(data, &reply) == nil && reply.Message != "" && len(reply.Message) <= 4096 {
				message = reply.Message
			}
		}
		writeAPIError(w, r, status, "frigate_upstream_error", message, nil)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if kind == "video" {
		media, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
		if media != "video/mp4" {
			writeAPIError(w, r, 502, "frigate_invalid_response", "Frigate returned an invalid clip", nil)
			return
		}
		w.Header().Set("Content-Type", "video/mp4")
		for _, key := range []string{"Content-Length", "Content-Range", "Accept-Ranges"} {
			if value := response.Header.Get(key); value != "" {
				w.Header().Set(key, value)
			}
		}
		w.WriteHeader(response.StatusCode)
		_, _ = io.Copy(w, response.Body)
		return
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, (8<<20)+1))
	if err != nil || len(payload) > 8<<20 {
		writeAPIError(w, r, 502, "frigate_invalid_response", "Frigate response exceeded its size limit", nil)
		return
	}
	if kind == "image" {
		media, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
		if media != "image/jpeg" || len(payload) < 3 || payload[0] != 0xff || payload[1] != 0xd8 {
			writeAPIError(w, r, 502, "frigate_invalid_response", "Frigate returned an invalid JPEG", nil)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.WriteHeader(200)
		_, _ = w.Write(payload)
		return
	}
	switch strings.Split(path, "?")[0] {
	case "/api/version":
		value := strings.TrimSpace(string(payload))
		var decoded string
		if json.Unmarshal(payload, &decoded) == nil {
			value = decoded
		}
		writeJSON(w, 200, map[string]string{"version": value})
		return
	case "/api/config/raw":
		value := string(payload)
		var decoded string
		if json.Unmarshal(payload, &decoded) == nil {
			value = decoded
		}
		writeJSON(w, 200, map[string]string{"config": value})
		return
	}
	if !json.Valid(payload) {
		writeAPIError(w, r, 502, "frigate_invalid_response", "Frigate returned invalid JSON", nil)
		return
	}
	if mutation {
		s.security.RecordAudit(r.Context(), s.securityRequestContext(r), actor, "modules.frigate.config.saved", "module", frigate.ModuleID, "success", nil)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(response.StatusCode)
	_, _ = w.Write(payload)
}

func (s *server) frigateRequest(ctx context.Context, method, path string, body []byte, byteRange string) (*http.Response, error) {
	resolver := s.frigateEndpoint
	if resolver == nil {
		resolver = modules.ResolveFrigateEndpoint
	}
	address, err := resolver(ctx)
	if err != nil {
		return nil, err
	}
	ip, err := netip.ParseAddr(address)
	if err != nil || !ip.Is4() || !ip.IsPrivate() || ip.IsLoopback() {
		return nil, errors.New("invalid private Frigate container address")
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://"+net.JoinHostPort(address, "5000")+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json, image/jpeg, video/mp4, text/plain")
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	}
	if method == http.MethodGet && strings.HasSuffix(strings.Split(path, "?")[0], ".mp4") && len(byteRange) < 128 && strings.HasPrefix(byteRange, "bytes=") {
		req.Header.Set("Range", byteRange)
	}
	transport := &http.Transport{Proxy: nil, DisableKeepAlives: true, DialContext: (&net.Dialer{Timeout: 4 * time.Second}).DialContext, ResponseHeaderTimeout: 15 * time.Second}
	defer transport.CloseIdleConnections()
	var roundTrip http.RoundTripper = transport
	if s.frigateTransport != nil {
		roundTrip = s.frigateTransport
	}
	client := &http.Client{Transport: roundTrip, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	// No browser cookie, bearer token, X-CSRF-Token, user header, proxy header or
	// user-selected destination is copied into this administrative connection.
	return client.Do(req)
}
