//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/windowsclient"
	"golang.org/x/sys/windows"
)

func TestAuthenticatedClientUsesWindowsCredentialManager(t *testing.T) {
	t.Setenv(passwordEnv, "")
	username := fmt.Sprintf("credential-test-%d", time.Now().UnixNano())
	var gotPassword string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/login" {
			http.NotFound(w, r)
			return
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		gotPassword = body["password"]
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"token-from-credential-store"}`))
	}))
	defer server.Close()

	err := windowsclient.SavePassword(server.URL, username, "stored-secret")
	if errors.Is(err, windows.ERROR_NO_SUCH_LOGON_SESSION) {
		t.Skipf("Windows runner has no credential set: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = windowsclient.DeletePassword(server.URL, username) })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := authenticatedClient(ctx, authFlags{server: server.URL, username: username})
	if err != nil {
		t.Fatal(err)
	}
	if client.Token != "token-from-credential-store" || gotPassword != "stored-secret" {
		t.Fatalf("token=%q password=%q", client.Token, gotPassword)
	}
}
