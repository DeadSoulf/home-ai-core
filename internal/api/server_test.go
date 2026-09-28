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

	"github.com/DeadSoulf/home-ai-core/internal/realtime"
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

func testHandler(state fakeState) http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(
		"00000000-0000-4000-8000-000000000000",
		logger,
		state,
		realtime.New("00000000-0000-4000-8000-000000000000", logger),
	)
}

func TestHealth(t *testing.T) {
	handler := testHandler(fakeState{schemaVersion: 1})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Header().Get("X-Request-ID") == "" {
		t.Fatal("missing X-Request-ID")
	}
	if rec.Header().Get("X-Correlation-ID") == "" {
		t.Fatal("missing X-Correlation-ID")
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("status body = %#v", body["status"])
	}
}

func TestCorrelationIDIsEchoed(t *testing.T) {
	handler := testHandler(fakeState{schemaVersion: 1})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("X-Correlation-ID", "mobile-upload-42")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Correlation-ID"); got != "mobile-upload-42" {
		t.Fatalf("correlation id = %q", got)
	}
}

func TestInvalidCorrelationIDFallsBackToRequestID(t *testing.T) {
	handler := testHandler(fakeState{schemaVersion: 1})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("X-Correlation-ID", "contains spaces")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Header().Get("X-Correlation-ID") != rec.Header().Get("X-Request-ID") {
		t.Fatal("invalid correlation ID was not replaced")
	}
}

func TestHealthFailsWhenStateIsUnavailable(t *testing.T) {
	handler := testHandler(fakeState{pingErr: errors.New("offline")})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestSystem(t *testing.T) {
	const nodeID = "00000000-0000-4000-8000-000000000000"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := New(
		nodeID,
		logger,
		fakeState{schemaVersion: 1},
		realtime.New(nodeID, logger),
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

func TestAPIErrorEnvelope(t *testing.T) {
	handler := testHandler(fakeState{schemaErr: errors.New("database unavailable")})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system", nil)
	req.Header.Set("X-Correlation-ID", "diagnostic-1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}

	var body errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if body.Error.Code != "state_unavailable" {
		t.Fatalf("error code = %q", body.Error.Code)
	}
	if body.Error.RequestID == "" {
		t.Fatal("missing error request ID")
	}
	if body.Error.CorrelationID != "diagnostic-1" {
		t.Fatalf("correlation ID = %q", body.Error.CorrelationID)
	}
}

func TestNotFoundUsesErrorEnvelope(t *testing.T) {
	handler := testHandler(fakeState{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/missing", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}

	var body errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if body.Error.Code != "not_found" {
		t.Fatalf("error code = %q", body.Error.Code)
	}
}
