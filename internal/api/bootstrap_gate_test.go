package api

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DeadSoulf/home-ai-core/internal/realtime"
	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/state"
)

func TestLANCannotBootstrapWithoutConsoleCode(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := state.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	sec, err := security.New(ctx, db, dir)
	if err != nil {
		t.Fatal(err)
	}
	code, err := os.ReadFile(filepath.Join(dir, "bootstrap-token"))
	if err != nil {
		t.Fatal(err)
	}
	s := &server{security: sec, realtime: realtime.New("node", slog.New(slog.NewTextHandler(io.Discard, nil)))}
	send := func(token string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]string{"username": "owner", "display_name": "Owner", "password": "correct owner passphrase", "session_mode": "cookie", "bootstrap_token": token})
		req := httptest.NewRequest(http.MethodPost, "https://localhost:82/api/v1/security/bootstrap", strings.NewReader(string(body)))
		req.RemoteAddr = "192.168.1.50:12345"
		req.TLS = &tls.ConnectionState{}
		w := httptest.NewRecorder()
		s.bootstrap(w, req)
		return w
	}
	for _, token := range []string{"", "bad-token"} {
		if w := send(token); w.Code != http.StatusForbidden {
			t.Fatalf("unauthorized LAN bootstrap: %d %s", w.Code, w.Body.String())
		}
	}
	if ready, _ := sec.Initialized(ctx); ready {
		t.Fatal("bad code created owner")
	}
	w := send(strings.TrimSpace(string(code)))
	if w.Code != http.StatusCreated {
		t.Fatalf("authorized setup failed %d %s", w.Code, w.Body.String())
	}
	for _, cookie := range w.Result().Cookies() {
		if !cookie.Secure || !cookie.HttpOnly {
			t.Fatal("TLS session cookie flags missing")
		}
	}
	if w := send(strings.TrimSpace(string(code))); w.Code != http.StatusConflict {
		t.Fatal("setup replay not rejected")
	}
}
