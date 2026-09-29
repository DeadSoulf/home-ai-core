package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/updater"
)

type UpdaterService interface {
	Check(context.Context) (updater.ReleaseStatus, error)
	State() updater.State
	Download(context.Context, string) (updater.State, error)
	Install(context.Context, string) (updater.State, error)
	Rollback(context.Context) (updater.State, error)
}

func (s *server) updateStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, r, http.MethodGet)
		return
	}
	if s.updater == nil {
		writeAPIError(w, r, http.StatusServiceUnavailable, "updater_unavailable", "updater is unavailable", nil)
		return
	}
	status, err := s.updater.Check(r.Context())
	if err != nil {
		s.logger.Error("update check failed", "error", err)
		writeAPIError(w, r, http.StatusBadGateway, "update_check_failed", "failed to check GitHub for updates", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"update": status})
}

func (s *server) updateState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, r, http.MethodGet)
		return
	}
	if s.updater == nil {
		writeAPIError(w, r, http.StatusServiceUnavailable, "updater_unavailable", "updater is unavailable", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"state": s.updater.State()})
}

func (s *server) updateDownload(
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
	if s.updater == nil {
		writeAPIError(w, r, http.StatusServiceUnavailable, "updater_unavailable", "updater is unavailable", nil)
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

	state, err := s.updater.Download(r.Context(), strings.TrimSpace(input.Version))
	switch {
	case errors.Is(err, updater.ErrBusy):
		writeAPIError(w, r, http.StatusConflict, "update_busy", err.Error(), nil)
	case errors.Is(err, updater.ErrNoUpdate):
		writeAPIError(w, r, http.StatusConflict, "update_not_available", "requested update is not available", nil)
	case err != nil:
		s.logger.Error("update download failed", "error", err)
		writeAPIError(w, r, http.StatusBadGateway, "update_download_failed", err.Error(), nil)
	default:
		writeJSON(w, http.StatusOK, map[string]any{"state": state})
	}
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
	if s.updater == nil {
		writeAPIError(w, r, http.StatusServiceUnavailable, "updater_unavailable", "updater is unavailable", nil)
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

	state, err := s.updater.Install(r.Context(), strings.TrimSpace(input.Version))
	switch {
	case errors.Is(err, updater.ErrBusy):
		writeAPIError(w, r, http.StatusConflict, "update_busy", err.Error(), nil)
	case errors.Is(err, updater.ErrHelperUpgradeRequired):
		writeAPIError(w, r, http.StatusConflict, "helper_upgrade_required", err.Error(), nil)
	case err != nil:
		s.logger.Error("update install failed", "error", err)
		writeAPIError(w, r, http.StatusBadGateway, "update_install_failed", err.Error(), nil)
	default:
		writeJSON(w, http.StatusAccepted, map[string]any{"state": state})
	}
}


func (s *server) updateRollback(
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
	if s.updater == nil {
		writeAPIError(w, r, http.StatusServiceUnavailable, "updater_unavailable", "updater is unavailable", nil)
		return
	}

	state, err := s.updater.Rollback(r.Context())
	switch {
	case errors.Is(err, updater.ErrBusy):
		writeAPIError(w, r, http.StatusConflict, "update_busy", err.Error(), nil)
	case errors.Is(err, updater.ErrNoRollback):
		writeAPIError(w, r, http.StatusConflict, "rollback_not_available", err.Error(), nil)
	case errors.Is(err, updater.ErrHelperUpgradeRequired):
		writeAPIError(w, r, http.StatusConflict, "helper_upgrade_required", err.Error(), nil)
	case err != nil:
		s.logger.Error("update rollback failed", "error", err)
		writeAPIError(w, r, http.StatusBadGateway, "update_rollback_failed", err.Error(), nil)
	default:
		s.security.RecordAudit(
			r.Context(),
			s.securityRequestContext(r),
			actor,
			"update.rollback",
			"core",
			state.AvailableVersion,
			"success",
			map[string]any{"target_version": state.AvailableVersion},
		)
		writeJSON(w, http.StatusAccepted, map[string]any{"state": state})
	}
}
