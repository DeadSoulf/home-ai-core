package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/realtime"
	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/systeminfo"
	"github.com/DeadSoulf/home-ai-core/internal/version"
)

type server struct {
	nodeID   string
	logger   *slog.Logger
	state    State
	security SecurityService
	realtime *realtime.Hub
	mux      *http.ServeMux
}

func New(
	nodeID string,
	logger *slog.Logger,
	state State,
	securityService SecurityService,
	realtimeHub *realtime.Hub,
) http.Handler {
	s := &server{
		nodeID:   nodeID,
		logger:   logger,
		state:    state,
		security: securityService,
		realtime: realtimeHub,
		mux:      http.NewServeMux(),
	}

	s.mux.HandleFunc("GET /health", s.health)
	s.mux.HandleFunc("/api/v1/security/setup-status", s.setupStatus)
	s.mux.HandleFunc("/api/v1/security/bootstrap", s.bootstrap)
	s.mux.HandleFunc("/api/v1/auth/login", s.login)
	s.mux.HandleFunc("/api/v1/auth/me", s.requireAuth(
		"security.self.read",
		func(w http.ResponseWriter, r *http.Request, actor security.Actor, _ authSource) {
			s.me(w, r, actor)
		},
	))
	s.mux.HandleFunc("/api/v1/auth/logout", s.requireAuth("", s.logout))
	s.mux.HandleFunc("/api/v1/system", s.requireAuth(
		"system.read",
		func(w http.ResponseWriter, r *http.Request, _ security.Actor, _ authSource) {
			s.systemRoute(w, r)
		},
	))
	s.mux.HandleFunc("/api/v1/events", s.requireAuth(
		"events.read",
		func(w http.ResponseWriter, r *http.Request, _ security.Actor, _ authSource) {
			s.eventsRoute(w, r)
		},
	))
	s.mux.HandleFunc("/api/v1/audit", s.requireAuth(
		"audit.read",
		func(w http.ResponseWriter, r *http.Request, actor security.Actor, _ authSource) {
			s.audit(w, r, actor)
		},
	))
	s.mux.HandleFunc("/", s.notFound)

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

func (s *server) systemRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, r, http.MethodGet)
		return
	}
	s.system(w, r)
}

func (s *server) system(w http.ResponseWriter, r *http.Request) {
	schemaVersion, err := s.state.SchemaVersion(r.Context())
	if err != nil {
		s.logger.Error("failed to read schema version", "error", err)
		writeAPIError(
			w,
			r,
			http.StatusInternalServerError,
			"state_unavailable",
			"core state is unavailable",
			nil,
		)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"version":        version.Version,
		"schema_version": schemaVersion,
		"system":         systeminfo.Collect(s.nodeID),
	})
}

func (s *server) eventsRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, r, http.MethodGet)
		return
	}
	s.events(w, r)
}

func (s *server) events(w http.ResponseWriter, r *http.Request) {
	s.realtime.ServeHTTP(w, r, requestIDFromContext(r.Context()))
}

func (s *server) notFound(w http.ResponseWriter, r *http.Request) {
	writeAPIError(
		w,
		r,
		http.StatusNotFound,
		"not_found",
		"resource not found",
		nil,
	)
}

func (s *server) requestContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r, meta := withRequestMetadata(r)
		w.Header().Set("X-Request-ID", meta.RequestID)
		w.Header().Set("X-Correlation-ID", meta.CorrelationID)

		started := time.Now()
		next.ServeHTTP(w, r)

		s.logger.Info("http request",
			"request_id", meta.RequestID,
			"correlation_id", meta.CorrelationID,
			"method", r.Method,
			"path", r.URL.Path,
			"remote", r.RemoteAddr,
			"duration_ms", time.Since(started).Milliseconds(),
		)
	})
}

var contextWithTimeout = context.WithTimeout
