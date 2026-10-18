package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/modules"
	"github.com/DeadSoulf/home-ai-core/internal/security"
)

const aiTestToken = "private_test_Agent_Token_ABC123456789"

func aiTestService(enabled bool) fakeModules {
	status := "disabled"
	if enabled {
		status = "enabled"
	}
	return fakeModules{items: []modules.Registered{{
		Manifest: modules.Manifest{ID: "ai.agent", Name: "AI Agent", Version: "0.1.0"},
		Status:   status,
	}}}
}

func aiTestServer(t *testing.T, upstream func(*http.Request)) *server {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "access-token"), []byte(aiTestToken+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return &server{
		modules:    aiTestService(true),
		security:   defaultFakeSecurity(),
		aiDataRoot: dir,
		aiEndpoint: func(context.Context) (string, error) { return "172.20.0.6", nil },
		aiTransport: nvrRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			if upstream != nil {
				upstream(r)
			}
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}},
				Body: io.NopCloser(strings.NewReader("{\"version\":\"0.1.0\",\"providers\":1,\"approvals_pending\":0,\"core_actions_enabled\":false}"))}, nil
		}),
	}
}

func TestAIGatewayOnlyPassesPrivateServerToken(t *testing.T) {
	calls := 0
	s := aiTestServer(t, func(r *http.Request) {
		calls++
		if r.URL.Host != "172.20.0.6:8080" || r.URL.Path != "/api/status" {
			t.Errorf("unsafe upstream: %s", r.URL)
		}
		if r.Header.Get("Authorization") != "Bearer "+aiTestToken {
			t.Error("missing server-to-module authentication")
		}
		if r.Header.Get("Cookie") != "" || r.Header.Get("X-CSRF-Token") != "" {
			t.Error("browser headers forwarded")
		}
	})
	req := httptest.NewRequest("GET", "/api/v1/ai/status", nil)
	req.Header.Set("Cookie", "session=browser_private")
	req.Header.Set("Authorization", "Bearer BROWSER_KEY")
	w := httptest.NewRecorder()
	s.aiGateway(w, req, security.Actor{}, authBearer)
	if w.Code != 200 || calls != 1 || strings.Contains(w.Body.String(), aiTestToken) {
		t.Fatalf("AI response: %d %s", w.Code, w.Body.String())
	}
}

func TestAIAdminOnlyAndDisabledState(t *testing.T) {
	sec := defaultFakeSecurity()
	sec.actor.Permissions = []string{"modules.read", "security.self.read"}
	handler := uiAPI(sec, &uiJobService{}, uiModuleService{fakeModules: aiTestService(true)})
	if got := uiRequest(handler, "GET", "/api/v1/ai/status", ""); got.Code != 403 {
		t.Fatalf("read-only user accessed shared AI: %d", got.Code)
	}
	s := aiTestServer(t, func(*http.Request) { t.Error("disabled module contacted") })
	s.modules = aiTestService(false)
	w := httptest.NewRecorder()
	s.aiGateway(w, httptest.NewRequest("GET", "/api/v1/ai/status", nil), security.Actor{}, authBearer)
	if w.Code != 503 {
		t.Fatalf("disabled module exposed: %d", w.Code)
	}
}

func TestAIMutationsRejectInvalidCookieCSRF(t *testing.T) {
	s := aiTestServer(t, func(*http.Request) { t.Error("CSRF mutation forwarded") })
	for _, tt := range []struct{ method, path, body string }{
		{"POST", "/api/v1/ai/chat", "{}"},
		{"POST", "/api/v1/ai/providers", "{}"},
		{"DELETE", "/api/v1/ai/providers/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ""},
		{"POST", "/api/v1/ai/approvals/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/decision", "{}"},
	} {
		w := httptest.NewRecorder()
		s.aiGateway(w, httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body)), security.Actor{}, authCookie)
		if w.Code != 403 {
			t.Errorf("CSRF bypass: %s %s: %d", tt.method, tt.path, w.Code)
		}
	}
}

func TestAIGatewayRejectsUnknownPaths(t *testing.T) {
	s := aiTestServer(t, func(*http.Request) { t.Error("unexpected upstream request") })
	for _, path := range []string{
		"/api/v1/ai/../etc/passwd",
		"/api/v1/ai/providers/notvalid",
		"/api/v1/ai/conversations/../messages",
		"/api/v1/ai/admin",
		"/api/v1/ai/health",
	} {
		w := httptest.NewRecorder()
		s.aiGateway(w, httptest.NewRequest("GET", path, nil), security.Actor{}, authBearer)
		if w.Code != 404 {
			t.Errorf("path %q returned %d", path, w.Code)
		}
	}
}

func TestAIAddressAndTokenCannotEscapeManagedArea(t *testing.T) {
	s := aiTestServer(t, nil)
	for _, ip := range []string{"127.0.0.1", "8.8.8.8", "localhost", "::1"} {
		s.aiEndpoint = func(context.Context) (string, error) { return ip, nil }
		if _, err := s.aiAddress(context.Background()); err == nil {
			t.Fatalf("unsafe endpoint accepted: %s", ip)
		}
	}
	tokenPath := filepath.Join(s.aiDataRoot, "access-token")
	if err := os.Chmod(tokenPath, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.aiToken(); err == nil {
		t.Fatal("world-readable token accepted")
	}
	outside := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(outside, []byte(aiTestToken), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(tokenPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, tokenPath); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := s.aiToken(); err == nil {
		t.Fatal("token symlink escape accepted")
	}
}

func TestAIRequestAndResponseSizes(t *testing.T) {
	s := aiTestServer(t, func(*http.Request) { t.Error("invalid input forwarded") })
	w := httptest.NewRecorder()
	s.aiGateway(w, httptest.NewRequest("POST", "/api/v1/ai/chat", strings.NewReader(strings.Repeat("x", aiMaxRequestBytes+1))), security.Actor{}, authBearer)
	if w.Code != 400 {
		t.Fatalf("oversized request accepted: %d", w.Code)
	}
	s.aiTransport = nvrRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(strings.Repeat("x", aiMaxReplyBytes+1)))}, nil
	})
	w = httptest.NewRecorder()
	s.aiGateway(w, httptest.NewRequest("GET", "/api/v1/ai/status", nil), security.Actor{}, authBearer)
	if w.Code != 502 {
		t.Fatalf("oversized response accepted: %d", w.Code)
	}
}

func TestAIChatOverridesGlobalHTTPWriteDeadline(t *testing.T) {
	s := aiTestServer(t, nil)
	s.aiTransport = nvrRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/api/chat" {
			t.Errorf("wrong AI upstream path: %s", req.URL.Path)
		}
		// Simulate cold inference longer than the Core HTTP listener's
		// short WriteTimeout. The chat's ResponseController must extend it.
		time.Sleep(180 * time.Millisecond)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"conversation_id":"abc","reply":"Привет!","approval_ids":[]}`)),
		}, nil
	})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.aiGateway(w, r, security.Actor{}, authBearer)
	})
	server := httptest.NewUnstartedServer(handler)
	server.Config.WriteTimeout = 60 * time.Millisecond
	server.Start()
	defer server.Close()
	client := &http.Client{Timeout: 2 * time.Second}
	request, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/ai/chat", strings.NewReader(`{"message":"Привет"}`))
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("Core disconnected slow AI chat before its response: %v", err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != 200 || !strings.Contains(string(data), "Привет!") {
		t.Fatalf("long AI chat response: %d %q %v", response.StatusCode, data, err)
	}
}

// The hardware inventory does not call the module or expose host routes.
func TestAIHardwareInventoryRequiresModuleManager(t *testing.T) {
	sec := defaultFakeSecurity()
	sec.actor.Permissions = []string{"modules.read", "security.self.read"}
	route := uiAPI(sec, &uiJobService{}, uiModuleService{fakeModules: aiTestService(true)})
	if got := uiRequest(route, "GET", "/api/v1/ai/hardware", ""); got.Code != http.StatusForbidden {
		t.Fatalf("read-only user accessed GPU inventory: %d", got.Code)
	}
	sec.actor.Permissions = append(sec.actor.Permissions, "modules.manage")
	route = uiAPI(sec, &uiJobService{}, uiModuleService{fakeModules: aiTestService(true)})
	res := uiRequest(route, "GET", "/api/v1/ai/hardware", "")
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "\"gpus\"") {
		t.Fatalf("manager cannot inspect GPU inventory: %d %s", res.Code, res.Body.String())
	}
}
