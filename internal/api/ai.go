package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/DeadSoulf/home-ai-core/internal/aiagent"
	"github.com/DeadSoulf/home-ai-core/internal/security"
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
