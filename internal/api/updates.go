package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/DeadSoulf/home-ai-core/internal/coreupdate"
	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/state"
)

type UpdateService interface {
	Check(context.Context) (coreupdate.Status, error)
	Install(context.Context, string, security.Actor, security.RequestContext) (state.JobRecord, error)
}

func (s *server) updatesCollection(
	w http.ResponseWriter,
	r *http.Request,
	_ security.Actor,
	_ authSource,
) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, r, http.MethodGet)
		return
	}
	if s.updates == nil {
		writeAPIError(w, r, http.StatusServiceUnavailable, "updates_unavailable", "update service is unavailable", nil)
		return
	}
	status, err := s.updates.Check(r.Context())
	if err != nil {
		writeAPIError(w, r, http.StatusBadGateway, "update_check_failed", "failed to check for updates", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"update": status})
}

func (s *server) updateInstall(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	source authSource,
) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, r, http.MethodPost)
		return
	}
	if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}
	if s.updates == nil {
		writeAPIError(w, r, http.StatusServiceUnavailable, "updates_unavailable", "update service is unavailable", nil)
		return
	}

	var input struct {
		Version string `json:"version"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || strings.TrimSpace(input.Version) == "" {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_update_request", "update version is required", nil)
		return
	}

	meta := s.securityRequestContext(r)
	job, err := s.updates.Install(r.Context(), strings.TrimSpace(input.Version), actor, meta)
	switch {
	case errors.Is(err, coreupdate.ErrNoUpdate):
		writeAPIError(w, r, http.StatusConflict, "no_update_available", "no update is available", nil)
	case err != nil:
		writeAPIError(w, r, http.StatusBadGateway, "update_install_failed", "failed to prepare update installation", nil)
	default:
		s.security.RecordAudit(
			r.Context(),
			meta,
			actor,
			"core.update.install",
			"job",
			job.ID,
			"accepted",
			map[string]any{"job_type": job.Type},
		)
		writeJSON(w, http.StatusAccepted, map[string]any{"job": job})
	}
}
