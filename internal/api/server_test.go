package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeState struct {
	pingErr       error
	schemaVersion int
	schemaErr     error
}

func (f fakeState) Ping(context.Context) error {
	return f.pingErr
}

func (f fakeState) SchemaVersion(context.Context) (int, error) {
	return f.schemaVersion, f.schemaErr
}

func TestHealth(t *testing.T) {
	handler := New(
		"00000000-0000-4000-8000-000000000000",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		fakeState{schemaVersion: 1},
	)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Header().Get("X-Request-ID") == "" {
		t.Fatal("missing X-Request-ID")
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("status body = %#v", body["status"])
	}
}

func TestHealthFailsWhenStateIsUnavailable(t *testing.T) {
	handler := New(
		"00000000-0000-4000-8000-000000000000",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		fakeState{pingErr: errors.New("offline")},
	)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestSystem(t *testing.T) {
	const nodeID = "00000000-0000-4000-8000-000000000000"
	handler := New(
		nodeID,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		fakeState{schemaVersion: 1},
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body struct {
		SchemaVersion int `json:"schema_version"`
		System        struct {
			NodeID string `json:"node_id"`
		} `json:"system"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.System.NodeID != nodeID {
		t.Fatalf("node_id = %q, want %q", body.System.NodeID, nodeID)
	}
	if body.SchemaVersion != 1 {
		t.Fatalf("schema_version = %d, want 1", body.SchemaVersion)
	}
}
