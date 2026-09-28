package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/security"
)

const (
	sessionCookieName = "home_ai_session"
	maxSecurityBody   = 64 << 10
)

type SecurityService interface {
	Initialized(context.Context) (bool, error)
	Bootstrap(context.Context, string, string, string, string, security.RequestContext) (security.AuthResult, error)
	Login(context.Context, string, string, security.RequestContext) (security.AuthResult, error)
	Authenticate(context.Context, string) (security.Actor, error)
	Logout(context.Context, security.Actor, security.RequestContext) error
	ListAudit(context.Context, int) ([]security.AuditEntry, error)
	RecordAudit(context.Context, security.RequestContext, security.Actor, string, string, string, string, map[string]any)
}

func (s *server) setupStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, r, http.MethodGet)
		return
	}
	initialized, err := s.security.Initialized(r.Context())
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "security_unavailable", "security state is unavailable", nil)
		return
	}
	bootstrap := "local_token_required"
	if initialized {
		bootstrap = "disabled"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"initialized": initialized,
		"bootstrap":   bootstrap,
	})
}

func (s *server) bootstrap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, r, http.MethodPost)
		return
	}
	if !remoteIsLoopback(r.RemoteAddr) {
		writeAPIError(w, r, http.StatusForbidden, "bootstrap_local_only", "bootstrap is available only from the local host", nil)
		return
	}

	var request struct {
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Password    string `json:"password"`
		SessionMode string `json:"session_mode,omitempty"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	mode, ok := validSessionMode(request.SessionMode)
	if !ok {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_session_mode", "session_mode must be cookie or token", nil)
		return
	}
	request.SessionMode = mode

	result, err := s.security.Bootstrap(
		r.Context(),
		r.Header.Get("X-Home-AI-Bootstrap-Token"),
		request.Username,
		request.DisplayName,
		request.Password,
		s.securityRequestContext(r),
	)
	if err != nil {
		switch {
		case errors.Is(err, security.ErrInvalidBootstrapToken):
			writeAPIError(w, r, http.StatusUnauthorized, "invalid_bootstrap_token", "invalid bootstrap token", nil)
		case errors.Is(err, security.ErrAlreadyInitialized):
			writeAPIError(w, r, http.StatusConflict, "already_initialized", "security is already initialized", nil)
		default:
			writeAPIError(w, r, http.StatusBadRequest, "bootstrap_failed", err.Error(), nil)
		}
		return
	}

	if !s.writeAuthResult(w, r, result, request.SessionMode, http.StatusCreated) {
		return
	}
	s.realtime.Publish("security.initialized", map[string]any{"user_id": result.Actor.ID}, requestIDFromContext(r.Context()))
}

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, r, http.MethodPost)
		return
	}

	var request struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		SessionMode string `json:"session_mode,omitempty"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	mode, ok := validSessionMode(request.SessionMode)
	if !ok {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_session_mode", "session_mode must be cookie or token", nil)
		return
	}
	request.SessionMode = mode

	result, err := s.security.Login(r.Context(), request.Username, request.Password, s.securityRequestContext(r))
	if err != nil {
		if errors.Is(err, security.ErrInvalidCredentials) {
			writeAPIError(w, r, http.StatusUnauthorized, "invalid_credentials", "invalid username or password", nil)
			return
		}
		writeAPIError(w, r, http.StatusInternalServerError, "authentication_unavailable", "authentication is unavailable", nil)
		return
	}

	s.writeAuthResult(w, r, result, request.SessionMode, http.StatusOK)
}

func (s *server) me(w http.ResponseWriter, r *http.Request, actor security.Actor) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, r, http.MethodGet)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"actor": actor})
}

func (s *server) logout(w http.ResponseWriter, r *http.Request, actor security.Actor, source authSource) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, r, http.MethodPost)
		return
	}
	if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}
	if err := s.security.Logout(r.Context(), actor, s.securityRequestContext(r)); err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "logout_failed", "failed to revoke session", nil)
		return
	}
	clearSessionCookie(w, r)
	w.WriteHeader(http.StatusNoContent)
	s.realtime.Publish("security.session.revoked", map[string]any{"user_id": actor.ID}, requestIDFromContext(r.Context()))
}

func (s *server) audit(w http.ResponseWriter, r *http.Request, _ security.Actor) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, r, http.MethodGet)
		return
	}
	limit := 100
	if value := r.URL.Query().Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed <= 0 || parsed > 500 {
			writeAPIError(w, r, http.StatusBadRequest, "invalid_limit", "limit must be between 1 and 500", nil)
			return
		}
		limit = parsed
	}
	entries, err := s.security.ListAudit(r.Context(), limit)
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "audit_unavailable", "audit log is unavailable", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": entries})
}

type authSource int

const (
	authNone authSource = iota
	authBearer
	authCookie
)

func (s *server) requireAuth(
	permission string,
	next func(http.ResponseWriter, *http.Request, security.Actor, authSource),
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, source := sessionToken(r)
		actor, err := s.security.Authenticate(r.Context(), token)
		if err != nil {
			if errors.Is(err, security.ErrUnauthorized) {
				writeAPIError(w, r, http.StatusUnauthorized, "authentication_required", "authentication required", nil)
				return
			}
			writeAPIError(w, r, http.StatusServiceUnavailable, "authentication_unavailable", "authentication is unavailable", nil)
			return
		}
		if permission != "" && !actor.Has(permission) {
			writeAPIError(w, r, http.StatusForbidden, "permission_denied", "permission denied", nil)
			return
		}
		next(w, r, actor, source)
	}
}

func sessionToken(r *http.Request) (string, authSource) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(header, "Bearer ") {
		token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		if token != "" {
			return token, authBearer
		}
	}
	if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
		return cookie.Value, authCookie
	}
	return "", authNone
}

func validSessionMode(mode string) (string, bool) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		mode = "cookie"
	}
	return mode, mode == "cookie" || mode == "token"
}

func (s *server) writeAuthResult(
	w http.ResponseWriter,
	r *http.Request,
	result security.AuthResult,
	mode string,
	status int,
) bool {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		mode = "cookie"
	}

	response := map[string]any{
		"actor":      result.Actor,
		"expires_at": result.ExpiresAt,
	}
	switch mode {
	case "cookie":
		setSessionCookie(w, r, result.Token, result.ExpiresAt)
		response["csrf_token"] = result.CSRFToken
	case "token":
		response["token"] = result.Token
	default:
		writeAPIError(w, r, http.StatusBadRequest, "invalid_session_mode", "session_mode must be cookie or token", nil)
		return false
	}
	writeJSON(w, status, response)
	return true
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, token string, expiresAt time.Time) {
	maxAge := int(time.Until(expiresAt).Seconds())
	if maxAge < 1 {
		maxAge = 1
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteStrictMode,
	})
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteStrictMode,
	})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxSecurityBody)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errors.New("invalid JSON request body")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain one JSON value")
	}
	return nil
}

func remoteIsLoopback(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

func methodNotAllowed(w http.ResponseWriter, r *http.Request, allowed string) {
	w.Header().Set("Allow", allowed)
	writeAPIError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
}

func (s *server) securityRequestContext(r *http.Request) security.RequestContext {
	meta := metadataFromContext(r.Context())
	return security.RequestContext{
		RequestID:     meta.RequestID,
		CorrelationID: meta.CorrelationID,
		RemoteAddr:    r.RemoteAddr,
	}
}
