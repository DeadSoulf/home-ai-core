package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/DeadSoulf/home-ai-core/internal/security"
)

type userLifecycleService interface {
	UpdateUserIdentity(context.Context, security.Actor, string, string, string, security.RequestContext) (security.User, error)
	ResetUserPassword(context.Context, security.Actor, string, string, security.RequestContext) error
	ChangePassword(context.Context, security.Actor, string, string, security.RequestContext) error
}

func (s *server) userIdentity(w http.ResponseWriter, r *http.Request, actor security.Actor, source authSource) {
	service, ok := s.userLifecycle(w, r, actor, source)
	if !ok {
		return
	}
	var input struct {
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeAPIError(w, r, 400, "invalid_request", err.Error(), nil)
		return
	}
	user, err := service.UpdateUserIdentity(r.Context(), actor, r.PathValue("userID"), input.Username, input.DisplayName, s.securityRequestContext(r))
	if err != nil {
		writeUserLifecycleError(w, r, err)
		return
	}
	s.realtime.Publish("security.user.identity.changed", map[string]any{"user_id": user.ID}, requestIDFromContext(r.Context()))
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (s *server) userPassword(w http.ResponseWriter, r *http.Request, actor security.Actor, source authSource) {
	service, ok := s.userLifecycle(w, r, actor, source)
	if !ok {
		return
	}
	var input struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeAPIError(w, r, 400, "invalid_request", err.Error(), nil)
		return
	}
	if err := service.ResetUserPassword(r.Context(), actor, r.PathValue("userID"), input.Password, s.securityRequestContext(r)); err != nil {
		writeUserLifecycleError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) accountPassword(w http.ResponseWriter, r *http.Request, actor security.Actor, source authSource) {
	service, ok := s.userLifecycle(w, r, actor, source)
	if !ok {
		return
	}
	var input struct {
		CurrentPassword string `json:"current_password"`
		Password        string `json:"password"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeAPIError(w, r, 400, "invalid_request", err.Error(), nil)
		return
	}
	if err := service.ChangePassword(r.Context(), actor, input.CurrentPassword, input.Password, s.securityRequestContext(r)); err != nil {
		writeUserLifecycleError(w, r, err)
		return
	}
	clearSessionCookie(w, r)
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) userLifecycle(w http.ResponseWriter, r *http.Request, actor security.Actor, source authSource) (userLifecycleService, bool) {
	if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
		writeAPIError(w, r, 403, "csrf_required", "valid CSRF token required", nil)
		return nil, false
	}
	service, ok := s.security.(userLifecycleService)
	if !ok {
		writeAPIError(w, r, 503, "user_management_unavailable", "user management unavailable", nil)
	}
	return service, ok
}

func writeUserLifecycleError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeAPIError(w, r, 404, "user_not_found", "user not found", nil)
	case errors.Is(err, security.ErrAdministratorRequired):
		writeAPIError(w, r, 403, "administrator_required", err.Error(), nil)
	case errors.Is(err, security.ErrInvalidCredentials):
		writeAPIError(w, r, 403, "invalid_current_password", "current password is incorrect", nil)
	case errors.Is(err, security.ErrUserExists):
		writeAPIError(w, r, 409, "user_exists", "username already exists", nil)
	default:
		writeAPIError(w, r, 400, "user_update_failed", err.Error(), nil)
	}
}
