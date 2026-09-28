package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/state"
)

type JobService interface {
	Get(context.Context, string) (state.JobRecord, error)
	List(context.Context, string, int) ([]state.JobRecord, error)
	Cancel(context.Context, string) (state.JobRecord, error)
}

func (s *server) jobsCollection(
	w http.ResponseWriter,
	r *http.Request,
	_ security.Actor,
	_ authSource,
) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, r, http.MethodGet)
		return
	}

	status := strings.TrimSpace(r.URL.Query().Get("status"))
	limit, ok := queryLimit(r, 100)
	if !ok {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_limit", "limit must be between 1 and 500", nil)
		return
	}

	jobs, err := s.jobs.List(r.Context(), status, limit)
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "jobs_unavailable", "jobs are unavailable", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": jobs})
}

func (s *server) jobResource(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	source authSource,
) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/jobs/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		s.notFound(w, r)
		return
	}
	id := parts[0]

	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, r, http.MethodGet)
			return
		}
		job, err := s.jobs.Get(r.Context(), id)
		if errors.Is(err, state.ErrJobNotFound) {
			writeAPIError(w, r, http.StatusNotFound, "job_not_found", "job not found", nil)
			return
		}
		if err != nil {
			writeAPIError(w, r, http.StatusInternalServerError, "jobs_unavailable", "jobs are unavailable", nil)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"job": job})
		return
	}

	if len(parts) == 2 && parts[1] == "cancel" {
		if r.Method != http.MethodPost {
			methodNotAllowed(w, r, http.MethodPost)
			return
		}
		if !actor.Has("jobs.cancel") {
			writeAPIError(w, r, http.StatusForbidden, "permission_denied", "permission denied", nil)
			return
		}
		if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
			writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
			return
		}

		job, err := s.jobs.Cancel(r.Context(), id)
		switch {
		case errors.Is(err, state.ErrJobNotFound):
			writeAPIError(w, r, http.StatusNotFound, "job_not_found", "job not found", nil)
		case errors.Is(err, state.ErrJobTerminal):
			writeAPIError(w, r, http.StatusConflict, "job_terminal", "job is already terminal", nil)
		case err != nil:
			writeAPIError(w, r, http.StatusInternalServerError, "job_cancel_failed", "failed to cancel job", nil)
		default:
			writeJSON(w, http.StatusAccepted, map[string]any{"job": job})
		}
		return
	}

	s.notFound(w, r)
}

func queryLimit(r *http.Request, fallback int) (int, bool) {
	value := strings.TrimSpace(r.URL.Query().Get("limit"))
	if value == "" {
		return fallback, true
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit <= 0 || limit > 500 {
		return 0, false
	}
	return limit, true
}
