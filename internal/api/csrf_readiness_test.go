package api

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/state"
)

func TestCookieCSRFReadinessAndFreshConsoleLogin(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := state.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	sec, err := security.New(ctx, store, dir)
	if err != nil {
		t.Fatal(err)
	}
	original, err := sec.Bootstrap(ctx, "owner", "Owner", "test-only-password-123", security.RequestContext{})
	if err != nil {
		t.Fatal(err)
	}
	s := &server{security: sec, console: newConsoleState()}
	me := s.requireAuth("", func(w http.ResponseWriter, r *http.Request, actor security.Actor, _ authSource) {
		s.me(w, r, actor)
	})
	ticket := s.requireAuth("system.console", s.consoleSession)
	check := func(cookie, csrf string, want bool) {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, "https://home-ai.test:82/api/v1/auth/me", nil)
		r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: cookie})
		r.Header.Set("X-CSRF-Token", csrf)
		w := httptest.NewRecorder()
		me(w, r)
		if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("session readiness status=%d, cache=%q", w.Code, w.Header().Get("Cache-Control"))
		}
		var body struct {
			Actor     security.Actor `json:"actor"`
			CSRFValid *bool          `json:"csrf_valid"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.CSRFValid == nil || *body.CSRFValid != want || body.Actor.ID != original.Actor.ID {
			t.Fatalf("unexpected readiness response: %#v", body)
		}
		if strings.Contains(w.Body.String(), cookie) || (csrf != "" && strings.Contains(w.Body.String(), csrf)) {
			t.Fatal("readiness response leaked a session or CSRF secret")
		}
	}
	check(original.Token, "", false)
	check(original.Token, "stale-token-from-another-origin", false)
	check(original.Token, original.CSRFToken, true)
	for _, csrf := range []string{"", "stale-token-from-another-origin"} {
		r := httptest.NewRequest(http.MethodPost, "https://home-ai.test:82/api/v1/system/console/session", nil)
		r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: original.Token})
		r.Header.Set("X-CSRF-Token", csrf)
		w := httptest.NewRecorder()
		ticket(w, r)
		if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "csrf_required") {
			t.Fatal("invalid CSRF obtained a console ticket")
		}
	}
	// Use the real login handler, a real SQLite session and the actual ticket handler.
	r := httptest.NewRequest(http.MethodPost, "https://home-ai.test:82/api/v1/auth/login",
		strings.NewReader(`{"username":"owner","password":"test-only-password-123","session_mode":"cookie"}`))
	r.TLS = &tls.ConnectionState{}
	w := httptest.NewRecorder()
	s.login(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("fresh login status=%d", w.Code)
	}
	var login struct {
		CSRFToken string `json:"csrf_token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &login); err != nil || login.CSRFToken == "" {
		t.Fatal("login did not return synchronizer token")
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly {
		t.Fatal("fresh login cookie must be Secure and HttpOnly")
	}
	check(cookies[0].Value, original.CSRFToken, false)
	check(cookies[0].Value, login.CSRFToken, true)
	check(original.Token, original.CSRFToken, true) // read-only probing never rotates/revokes sessions
	r = httptest.NewRequest(http.MethodPost, "https://home-ai.test:82/api/v1/system/console/session", nil)
	r.AddCookie(cookies[0])
	r.Header.Set("X-CSRF-Token", login.CSRFToken)
	w = httptest.NewRecorder()
	ticket(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("fresh login token could not obtain console ticket: status=%d", w.Code)
	}
	if len(s.console.tickets) != 1 {
		t.Fatal("unexpected number of issued console tickets")
	}
}

func TestSessionReadinessDoesNotRequireCSRFForBearerClients(t *testing.T) {
	s := &server{}
	r := httptest.NewRequest(http.MethodGet, "https://home-ai.test:82/api/v1/auth/me", nil)
	r.Header.Set("Authorization", "Bearer test-session")
	w := httptest.NewRecorder()
	s.me(w, r, security.Actor{ID: "test-user"})
	var body struct {
		CSRFValid bool `json:"csrf_valid"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || !body.CSRFValid {
		t.Fatal("bearer clients should not be required to carry cookie CSRF")
	}
}
