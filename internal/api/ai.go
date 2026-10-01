package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/DeadSoulf/home-ai-core/internal/aiagent"
	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/state"
)

func (s *server) aiStatus(w http.ResponseWriter, r *http.Request, _ security.Actor, _ authSource) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, r, http.MethodGet)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ai": s.ai.Status()})
}

func (s *server) aiModels(w http.ResponseWriter, r *http.Request, _ security.Actor, _ authSource) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, r, http.MethodGet)
		return
	}
	models, err := s.ai.Models(r.Context())
	if err != nil {
		writeAPIError(w, r, http.StatusServiceUnavailable, "ai_provider_unavailable", "local AI provider is unavailable", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"provider": s.ai.Status().ProviderID, "models": models})
}

func (s *server) aiTools(w http.ResponseWriter, r *http.Request, actor security.Actor, _ authSource) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, r, http.MethodGet)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tools": s.ai.AvailableTools(actor)})
}

func (s *server) aiToolResource(w http.ResponseWriter, r *http.Request, actor security.Actor, source authSource) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/ai/tools/"), "/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "execute" {
		s.notFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w, r, http.MethodPost)
		return
	}
	if !aiRequireCSRF(w, r, actor, source) {
		return
	}

	var request struct {
		Input    json.RawMessage `json:"input,omitempty"`
		Approved bool            `json:"approved,omitempty"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}

	result, err := s.ai.Execute(
		r.Context(),
		actor,
		s.securityRequestContext(r),
		parts[0],
		request.Input,
		request.Approved,
	)
	switch {
	case errors.Is(err, aiagent.ErrToolNotFound):
		writeAPIError(w, r, http.StatusNotFound, "ai_tool_not_found", "AI tool not found", nil)
	case errors.Is(err, aiagent.ErrPermissionDenied):
		writeAPIError(w, r, http.StatusForbidden, "permission_denied", "permission denied", nil)
	case errors.Is(err, aiagent.ErrApprovalRequired):
		writeAPIError(w, r, http.StatusConflict, "ai_approval_required", "explicit approval is required", nil)
	case errors.Is(err, aiagent.ErrInvalidToolInput):
		writeAPIError(w, r, http.StatusBadRequest, "invalid_ai_tool_input", "invalid AI tool input", nil)
	case err != nil:
		writeAPIError(w, r, http.StatusInternalServerError, "ai_tool_failed", "AI tool execution failed", nil)
	default:
		writeJSON(w, http.StatusOK, map[string]any{"tool_id": parts[0], "result": result})
	}
}

func (s *server) aiSessions(w http.ResponseWriter, r *http.Request, actor security.Actor, source authSource) {
	switch r.Method {
	case http.MethodGet:
		items, err := s.ai.Conversations(r.Context(), actor)
		if err != nil {
			writeAPIError(w, r, http.StatusServiceUnavailable, "ai_sessions_unavailable", "AI sessions are unavailable", nil)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"sessions": items})
	case http.MethodPost:
		if !aiRequireCSRF(w, r, actor, source) {
			return
		}
		var request struct {
			Model string `json:"model"`
		}
		if err := decodeJSON(w, r, &request); err != nil {
			writeAPIError(w, r, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		item, err := s.ai.CreateConversation(r.Context(), actor, request.Model)
		if err != nil {
			status, code, message := aiConversationError(err)
			writeAPIError(w, r, status, code, message, nil)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"session": item})
	default:
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodPost)
		writeAPIError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

func (s *server) aiSessionResource(w http.ResponseWriter, r *http.Request, actor security.Actor, source authSource) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/ai/sessions/"), "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		s.notFound(w, r)
		return
	}
	sessionID := parts[0]
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, r, http.MethodGet)
			return
		}
		item, err := s.ai.Conversation(r.Context(), actor, sessionID)
		if err != nil {
			status, code, message := aiConversationError(err)
			writeAPIError(w, r, status, code, message, nil)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"session": item})
		return
	}
	if len(parts) != 2 || parts[1] != "messages" {
		s.notFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w, r, http.MethodPost)
		return
	}
	if !aiRequireCSRF(w, r, actor, source) {
		return
	}
	var request struct {
		Content string `json:"content"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	if _, err := s.ai.Conversation(r.Context(), actor, sessionID); err != nil {
		status, code, message := aiConversationError(err)
		writeAPIError(w, r, status, code, message, nil)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeAPIError(w, r, http.StatusInternalServerError, "streaming_unavailable", "streaming is unavailable", nil)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	item, err := s.ai.StreamConversation(
		r.Context(),
		actor,
		s.securityRequestContext(r),
		sessionID,
		request.Content,
		func(delta string) error {
			if err := writeAIEvent(w, "delta", map[string]any{"content": delta}); err != nil {
				return err
			}
			flusher.Flush()
			return nil
		},
	)
	if err != nil {
		_, code, message := aiConversationError(err)
		_ = writeAIEvent(w, "error", map[string]any{"code": code, "message": message})
		flusher.Flush()
		return
	}
	_ = writeAIEvent(w, "done", map[string]any{"session": item})
	flusher.Flush()
}

func aiRequireCSRF(w http.ResponseWriter, r *http.Request, actor security.Actor, source authSource) bool {
	if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return false
	}
	return true
}

func aiConversationError(err error) (int, string, string) {
	switch {
	case errors.Is(err, state.ErrAISessionNotFound):
		return http.StatusNotFound, "ai_session_not_found", "AI session not found"
	case errors.Is(err, aiagent.ErrModelNotFound):
		return http.StatusBadRequest, "ai_model_not_found", "AI model not found"
	case errors.Is(err, aiagent.ErrMessageTooLarge):
		return http.StatusBadRequest, "ai_message_too_large", "AI message is too large"
	case errors.Is(err, aiagent.ErrInvalidToolInput):
		return http.StatusBadRequest, "invalid_ai_message", "AI message is required"
	case errors.Is(err, aiagent.ErrProviderUnavailable):
		return http.StatusServiceUnavailable, "ai_provider_unavailable", "local AI provider is unavailable"
	case errors.Is(err, aiagent.ErrConversationUnavailable):
		return http.StatusServiceUnavailable, "ai_sessions_unavailable", "AI sessions are unavailable"
	default:
		return http.StatusInternalServerError, "ai_chat_failed", "AI chat request failed"
	}
}

func writeAIEvent(w http.ResponseWriter, event string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, raw)
	return err
}
