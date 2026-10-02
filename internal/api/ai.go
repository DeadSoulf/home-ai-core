package api

import (
	"context"
	"encoding/json"
	"errors"
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
	if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
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
	case errors.Is(err, aiagent.ErrAgentDisabled):
		writeAPIError(w, r, http.StatusServiceUnavailable, "ai_agent_disabled", "AI Agent is disabled", nil)
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

func (s *server) aiConversations(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	source authSource,
) {
	switch r.Method {
	case http.MethodGet:
		items, err := s.ai.Conversations(r.Context(), actor)
		if err != nil {
			s.writeAIChatError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"conversations": items})
	case http.MethodPost:
		if !validMutationCSRF(actor, source, r) {
			writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
			return
		}
		var request struct {
			Title string `json:"title,omitempty"`
		}
		if err := decodeJSON(w, r, &request); err != nil {
			writeAPIError(w, r, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		item, err := s.ai.CreateConversation(r.Context(), actor, s.securityRequestContext(r), request.Title)
		if err != nil {
			s.writeAIChatError(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"conversation": item})
	default:
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodPost)
		writeAPIError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

func (s *server) aiConversationResource(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	source authSource,
) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/ai/conversations/"), "/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" {
		s.notFound(w, r)
		return
	}
	conversationID := parts[0]

	if parts[1] == "close" {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeAPIError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
			return
		}
		if !validMutationCSRF(actor, source, r) {
			writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
			return
		}
		conversation, err := s.ai.CloseConversation(
			r.Context(),
			actor,
			s.securityRequestContext(r),
			conversationID,
		)
		if err != nil {
			s.writeAIChatError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"conversation": conversation})
		return
	}
	if parts[1] != "messages" {
		s.notFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		messages, err := s.ai.Messages(r.Context(), actor, conversationID)
		if err != nil {
			s.writeAIChatError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"messages": messages})
	case http.MethodPost:
		if !validMutationCSRF(actor, source, r) {
			writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
			return
		}
		var request struct {
			Content      string `json:"content"`
			ProviderMode string `json:"provider_mode,omitempty"`
		}
		if err := decodeJSON(w, r, &request); err != nil {
			writeAPIError(w, r, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		userMessage, assistantMessage, err := s.ai.ChatWithProvider(
			r.Context(),
			actor,
			s.securityRequestContext(r),
			conversationID,
			request.Content,
			request.ProviderMode,
		)
		if err != nil {
			s.writeAIChatError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"user_message":      userMessage,
			"assistant_message": assistantMessage,
		})
	default:
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodPost)
		writeAPIError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

func (s *server) aiConversationMessageStream(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	source authSource,
) {
	if !validMutationCSRF(actor, source, r) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}
	conversationID := r.PathValue("conversationID")
	if conversationID == "" {
		s.notFound(w, r)
		return
	}
	var request struct {
		Content string `json:"content"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}

	started := false
	controller := http.NewResponseController(w)
	writeEvent := func(event any) error {
		if !started {
			w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache, no-transform")
			w.Header().Set("X-Accel-Buffering", "no")
			w.WriteHeader(http.StatusOK)
			started = true
		}
		if err := json.NewEncoder(w).Encode(event); err != nil {
			return err
		}
		return controller.Flush()
	}

	userMessage, assistantMessage, err := s.ai.ChatStream(
		r.Context(),
		actor,
		s.securityRequestContext(r),
		conversationID,
		request.Content,
		func(delta string) error {
			if delta == "" {
				return nil
			}
			return writeEvent(map[string]any{"type": "delta", "content": delta})
		},
	)
	if err != nil {
		if !started {
			s.writeAIChatError(w, r, err)
			return
		}
		_ = writeEvent(map[string]any{
			"type":    "error",
			"code":    "ai_stream_failed",
			"message": "AI response stream failed",
		})
		return
	}

	_ = writeEvent(map[string]any{
		"type":              "done",
		"user_message":      userMessage,
		"assistant_message": assistantMessage,
	})
}

func (s *server) aiConversationDelete(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	source authSource,
) {
	if !validMutationCSRF(actor, source, r) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}
	conversationID := r.PathValue("conversationID")
	if conversationID == "" {
		s.notFound(w, r)
		return
	}
	if err := s.ai.DeleteConversation(
		r.Context(),
		actor,
		s.securityRequestContext(r),
		conversationID,
	); err != nil {
		s.writeAIChatError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": conversationID})
}

func (s *server) aiClosedConversationsDelete(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	source authSource,
) {
	if !validMutationCSRF(actor, source, r) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}
	deleted, err := s.ai.ClearClosedConversations(
		r.Context(),
		actor,
		s.securityRequestContext(r),
	)
	if err != nil {
		s.writeAIChatError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted})
}

func (s *server) aiConversationActions(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	_ authSource,
) {
	items, err := s.ai.Actions(r.Context(), actor, r.PathValue("conversationID"))
	if err != nil {
		s.writeAIActionError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"actions": items})
}

func (s *server) aiConversationActionApprove(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	source authSource,
) {
	if !validMutationCSRF(actor, source, r) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}
	action, err := s.ai.ApproveAction(
		r.Context(),
		actor,
		s.securityRequestContext(r),
		r.PathValue("conversationID"),
		r.PathValue("actionID"),
	)
	if err != nil {
		s.writeAIActionError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"action": action})
}

func (s *server) aiConversationActionReject(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	source authSource,
) {
	if !validMutationCSRF(actor, source, r) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}
	action, err := s.ai.RejectAction(
		r.Context(),
		actor,
		s.securityRequestContext(r),
		r.PathValue("conversationID"),
		r.PathValue("actionID"),
	)
	if err != nil {
		s.writeAIActionError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"action": action})
}

func (s *server) writeAIActionError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, state.ErrAIToolActionNotFound):
		writeAPIError(w, r, http.StatusNotFound, "ai_action_not_found", "AI server action not found", nil)
	case errors.Is(err, state.ErrAIToolActionNotPending):
		writeAPIError(w, r, http.StatusConflict, "ai_action_not_pending", "AI server action is no longer pending", nil)
	case errors.Is(err, state.ErrAIConversationNotFound):
		writeAPIError(w, r, http.StatusNotFound, "ai_conversation_not_found", "AI conversation not found", nil)
	case errors.Is(err, state.ErrAIConversationClosed):
		writeAPIError(w, r, http.StatusConflict, "ai_conversation_closed", "AI conversation is closed", nil)
	case errors.Is(err, state.ErrAIConversationNotClosed):
		writeAPIError(w, r, http.StatusConflict, "ai_conversation_not_closed", "AI conversation must be finished before deletion", nil)
	case errors.Is(err, aiagent.ErrAgentDisabled):
		writeAPIError(w, r, http.StatusServiceUnavailable, "ai_agent_disabled", "AI Agent is disabled", nil)
	case errors.Is(err, aiagent.ErrPermissionDenied):
		writeAPIError(w, r, http.StatusForbidden, "permission_denied", "permission denied", nil)
	case errors.Is(err, aiagent.ErrInvalidToolInput):
		writeAPIError(w, r, http.StatusBadRequest, "invalid_ai_tool_input", "invalid AI tool input", nil)
	case errors.Is(err, context.DeadlineExceeded):
		writeAPIError(w, r, http.StatusGatewayTimeout, "ai_tool_timeout", "AI server action timed out", nil)
	case errors.Is(err, context.Canceled):
		writeAPIError(w, r, http.StatusRequestTimeout, "ai_action_cancelled", "AI server action was cancelled", nil)
	default:
		writeAPIError(w, r, http.StatusBadGateway, "ai_action_failed", "AI server action failed", nil)
	}
}

func validMutationCSRF(actor security.Actor, source authSource, r *http.Request) bool {
	return source != authCookie || actor.ValidCSRF(r.Header.Get("X-CSRF-Token"))
}

func (s *server) writeAIChatError(w http.ResponseWriter, r *http.Request, err error) {
	if s.logger != nil {
		s.logger.Warn(
			"AI chat request failed",
			"request_id", requestIDFromContext(r.Context()),
			"correlation_id", metadataFromContext(r.Context()).CorrelationID,
			"error", err,
		)
	}
	switch {
	case errors.Is(err, aiagent.ErrAgentDisabled):
		writeAPIError(w, r, http.StatusServiceUnavailable, "ai_agent_disabled", "AI Agent is disabled", nil)
	case errors.Is(err, state.ErrAIConversationNotFound):
		writeAPIError(w, r, http.StatusNotFound, "ai_conversation_not_found", "AI conversation not found", nil)
	case errors.Is(err, state.ErrAIConversationClosed):
		writeAPIError(w, r, http.StatusConflict, "ai_conversation_closed", "AI conversation is closed", nil)
	case errors.Is(err, state.ErrAIConversationNotClosed):
		writeAPIError(w, r, http.StatusConflict, "ai_conversation_not_closed", "AI conversation must be finished before deletion", nil)
	case errors.Is(err, aiagent.ErrChatUnavailable):
		writeAPIError(w, r, http.StatusServiceUnavailable, "ai_chat_unavailable", "AI chat is not configured or unavailable", nil)
	case errors.Is(err, aiagent.ErrInvalidChatMessage):
		writeAPIError(w, r, http.StatusBadRequest, "invalid_ai_message", "AI message is empty or too large", nil)
	case errors.Is(err, aiagent.ErrInvalidProviderMode):
		writeAPIError(w, r, http.StatusBadRequest, "invalid_ai_provider_mode", "AI provider mode is invalid", nil)
	case errors.Is(err, aiagent.ErrProviderUnavailable):
		writeAPIError(w, r, http.StatusServiceUnavailable, "ai_provider_unavailable", "requested AI provider is unavailable", nil)
	case errors.Is(err, context.DeadlineExceeded):
		writeAPIError(w, r, http.StatusGatewayTimeout, "ai_provider_timeout", "AI provider timed out", nil)
	case errors.Is(err, context.Canceled):
		writeAPIError(w, r, http.StatusRequestTimeout, "ai_request_cancelled", "AI request was cancelled", nil)
	default:
		var providerErr *aiagent.ProviderRequestError
		if errors.As(err, &providerErr) {
			status := http.StatusBadGateway
			code := "ai_provider_failed"
			switch providerErr.Kind {
			case "authentication":
				code = "cloud_ai_auth_failed"
			case "not_found":
				code = "cloud_ai_endpoint_or_model_not_found"
			case "rate_limit":
				status = http.StatusTooManyRequests
				code = "cloud_ai_rate_limited"
			case "invalid_request":
				code = "cloud_ai_invalid_request"
			case "network":
				status = http.StatusServiceUnavailable
				code = "cloud_ai_network_error"
			case "unavailable":
				status = http.StatusServiceUnavailable
				code = "cloud_ai_unavailable"
			}
			writeAPIError(w, r, status, code, providerErr.Error(), nil)
			return
		}
		writeAPIError(w, r, http.StatusBadGateway, "ai_provider_failed", "AI provider request failed", nil)
	}
}
