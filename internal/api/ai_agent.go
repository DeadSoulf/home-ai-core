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
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/modules"
	"github.com/DeadSoulf/home-ai-core/internal/security"
)

const (
	aiModuleID        = "ai.agent"
	defaultAIDataRoot = "/var/lib/home-ai-modules/ai.agent"
	aiMaxRequestBytes = 64 << 10
	aiMaxReplyBytes   = 4 << 20
)

var aiObjectID = regexp.MustCompile(`^[a-f0-9]{32}$`)

// The current AI module keeps one shared conversation database and provider
// credential store. Until per-user isolation is implemented, every AI route
// is intentionally restricted to modules.manage administrators.
func (s *server) aiGateway(w http.ResponseWriter, r *http.Request, actor security.Actor, source authSource) {
	method := r.Method
	subpath := strings.TrimPrefix(r.URL.Path, "/api/v1/ai/")
	path := ""
	auditAction := ""
	parts := strings.Split(subpath, "/")
	switch {
	case method == http.MethodGet && (subpath == "status" || subpath == "providers" ||
		subpath == "conversations" || subpath == "approvals"):
		path = "/api/" + subpath
	case method == http.MethodGet && len(parts) == 3 &&
		parts[0] == "conversations" && parts[2] == "messages" && aiObjectID.MatchString(parts[1]):
		path = "/api/conversations/" + parts[1] + "/messages"
	case method == http.MethodPost && subpath == "providers":
		path, auditAction = "/api/providers", "ai.provider.create"
	case method == http.MethodPost && subpath == "chat":
		path, auditAction = "/api/chat", "ai.chat.send"
	case method == http.MethodDelete && len(parts) == 2 &&
		parts[0] == "providers" && aiObjectID.MatchString(parts[1]):
		path, auditAction = "/api/providers/" + parts[1], "ai.provider.delete"
	case method == http.MethodPost && len(parts) == 3 &&
		parts[0] == "approvals" && parts[2] == "decision" && aiObjectID.MatchString(parts[1]):
		path, auditAction = "/api/approvals/" + parts[1] + "/decision", "ai.approval.decide"
	default:
		s.notFound(w, r)
		return
	}
	if method != http.MethodGet && source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}
	if s.modules == nil {
		writeAPIError(w, r, http.StatusServiceUnavailable, "ai_not_running", "AI module is unavailable", nil)
		return
	}
	item, err := s.modules.Get(r.Context(), aiModuleID)
	if err != nil || item.Status != "enabled" {
		writeAPIError(w, r, http.StatusServiceUnavailable, "ai_not_running", "AI Agent is not installed or enabled", nil)
		return
	}

	var payload []byte
	if method == http.MethodPost {
		r.Body = http.MaxBytesReader(w, r.Body, aiMaxRequestBytes)
		data, readErr := io.ReadAll(r.Body)
		if readErr != nil || len(data) == 0 || !json.Valid(data) {
			writeAPIError(w, r, http.StatusBadRequest, "invalid_ai_request", "Expected a valid JSON request body", nil)
			return
		}
		payload = data
	}

	timeout := 15 * time.Second
	if subpath == "chat" {
		timeout = 70 * time.Second // The local AI model can take up to 45s.
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	response, err := s.aiRequest(ctx, method, path, payload)
	if err != nil {
		writeAPIError(w, r, http.StatusBadGateway, "ai_unavailable", "Unable to contact the managed AI module", nil)
		return
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, aiMaxReplyBytes+1))
	if err != nil || len(body) > aiMaxReplyBytes || !json.Valid(body) ||
		!strings.HasPrefix(response.Header.Get("Content-Type"), "application/json") {
		writeAPIError(w, r, http.StatusBadGateway, "ai_invalid_response", "AI module returned invalid JSON", nil)
		return
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		status := http.StatusBadGateway
		message := "AI provider or module rejected the request"
		switch response.StatusCode {
		case http.StatusBadRequest:
			status, message = http.StatusBadRequest, "AI module rejected the input"
		case http.StatusNotFound:
			status, message = http.StatusNotFound, "AI resource not found"
		case http.StatusConflict:
			status, message = http.StatusConflict, "AI action was already decided or is unavailable"
		case http.StatusUnauthorized:
			message = "AI module authentication token was rejected"
		}
		writeAPIError(w, r, status, "ai_upstream_error", message, nil)
		return
	}
	if auditAction != "" {
		// Never put model prompts, replies, URLs, API keys or token content in audit.
		s.security.RecordAudit(r.Context(), s.securityRequestContext(r), actor,
			auditAction, "module", aiModuleID, "success", map[string]any{"method": method})
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(response.StatusCode)
	_, _ = w.Write(body)
}

func (s *server) aiAddress(ctx context.Context) (string, error) {
	resolver := s.aiEndpoint
	if resolver == nil {
		resolver = modules.ResolveAIEndpoint
	}
	address, err := resolver(ctx)
	if err != nil {
		return "", err
	}
	ip, err := netip.ParseAddr(address)
	if err != nil || !ip.Is4() || !ip.IsPrivate() || ip.IsLoopback() {
		return "", errors.New("invalid private AI container address")
	}
	return address, nil
}

func (s *server) aiToken() (string, error) {
	rootPath := s.aiDataRoot
	if rootPath == "" {
		rootPath = defaultAIDataRoot
	}
	// OpenRoot confines the token read to the managed module's persistent data
	// and refuses traversal through a symlink that escapes the root.
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return "", fmt.Errorf("open AI data root: %w", err)
	}
	defer root.Close()
	file, err := root.Open("access-token")
	if err != nil {
		return "", fmt.Errorf("open AI module token: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return "", errors.New("AI access token must be a private regular file")
	}
	data, err := io.ReadAll(io.LimitReader(file, 513))
	if err != nil || len(data) > 512 {
		return "", errors.New("AI access token is invalid or too large")
	}
	token := strings.TrimSpace(string(data))
	if len(token) < 24 || len(token) > 256 || strings.ContainsAny(token, "
") {
		return "", errors.New("invalid AI access token")
	}
	return token, nil
}

// This is not an arbitrary URL proxy. The target IP is helper-verified,
// port/path are hard-coded, only our own bearer token is forwarded, redirects
// are disabled, and neither browser cookies nor Authorization are propagated.
func (s *server) aiRequest(ctx context.Context, method, path string, data []byte) (*http.Response, error) {
	address, err := s.aiAddress(ctx)
	if err != nil {
		return nil, err
	}
	token, err := s.aiToken()
	if err != nil {
		return nil, err
	}
	var body io.Reader
	if method == http.MethodPost {
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method,
		"http://"+net.JoinHostPort(address, "8080")+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	transport := &http.Transport{
		Proxy: nil,
		DisableKeepAlives: true,
		DialContext: (&net.Dialer{Timeout: 4*time.Second}).DialContext,
		ResponseHeaderTimeout: 62*time.Second,
	}
	defer transport.CloseIdleConnections()
	var roundTrip http.RoundTripper = transport
	if s.aiTransport != nil {
		roundTrip = s.aiTransport
	}
	client := &http.Client{
		Transport: roundTrip,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return client.Do(req)
}
