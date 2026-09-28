package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/systeminfo"
	"github.com/DeadSoulf/home-ai-core/internal/version"
)

type server struct {
	nodeID string
	logger *slog.Logger
	state  State
	mux    *http.ServeMux
}

func New(nodeID string, logger *slog.Logger, state State) http.Handler {
	s := &server{
		nodeID: nodeID,
		logger: logger,
		state:  state,
		mux:    http.NewServeMux(),
	}

	s.mux.HandleFunc("GET /health", s.health)
	s.mux.HandleFunc("GET /api/v1/system", s.system)

	return s.requestContext(s.mux)
}

func (s *server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextWithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := s.state.Ping(ctx); err != nil {
		s.logger.Error("health check failed", "component", "state", "error", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"status":  "degraded",
			"version": version.Version,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"version": version.Version,
	})
}

func (s *server) system(w http.ResponseWriter, r *http.Request) {
	schemaVersion, err := s.state.SchemaVersion(r.Context())
	if err != nil {
		s.logger.Error("failed to read schema version", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"code":    "state_unavailable",
			"message": "core state is unavailable",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"version":        version.Version,
		"schema_version": schemaVersion,
		"system":         systeminfo.Collect(s.nodeID),
	})
}

func (s *server) requestContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := newRequestID()
		w.Header().Set("X-Request-ID", requestID)

		started := time.Now()
		next.ServeHTTP(w, r)

		s.logger.Info("http request",
			"request_id", requestID,
			"method", r.Method,
			"path", r.URL.Path,
			"remote", r.RemoteAddr,
			"duration_ms", time.Since(started).Milliseconds(),
		)
	})
}

func newRequestID() string {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "request-id-unavailable"
	}
	return hex.EncodeToString(raw[:])
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// Kept as a tiny seam for deterministic handler tests and to avoid repeating
// timeout boilerplate in health dependencies.
var contextWithTimeout = context.WithTimeout
