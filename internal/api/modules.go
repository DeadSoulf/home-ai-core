package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/DeadSoulf/home-ai-core/internal/modules"
	"github.com/DeadSoulf/home-ai-core/internal/security"
)

type ModuleService interface {
	List(context.Context) ([]modules.Registered, error)
	Get(context.Context, string) (modules.Registered, error)
	Capabilities(context.Context) ([]string, error)
	SetStatus(context.Context, string, string, string) error
}

func (s *server) modulesCollection(
	w http.ResponseWriter,
	r *http.Request,
	_ security.Actor,
	_ authSource,
) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, r, http.MethodGet)
		return
	}
	items, err := s.modules.List(r.Context())
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "modules_unavailable", "module registry is unavailable", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"modules": items})
}

func (s *server) moduleCapabilities(
	w http.ResponseWriter,
	r *http.Request,
	_ security.Actor,
	_ authSource,
) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, r, http.MethodGet)
		return
	}
	capabilities, err := s.modules.Capabilities(r.Context())
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "modules_unavailable", "module registry is unavailable", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"capabilities": capabilities})
}

func (s *server) moduleResource(
	w http.ResponseWriter,
	r *http.Request,
	_ security.Actor,
	_ authSource,
) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, r, http.MethodGet)
		return
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/modules/"), "/")
	if id == "" || strings.Contains(id, "/") {
		s.notFound(w, r)
		return
	}

	item, err := s.modules.Get(r.Context(), id)
	if errors.Is(err, modules.ErrModuleNotFound) {
		writeAPIError(w, r, http.StatusNotFound, "module_not_found", "module not found", nil)
		return
	}
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "modules_unavailable", "module registry is unavailable", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"module": item})
}

func (s *server) moduleControl(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	source authSource,
) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, r, http.MethodPost)
		return
	}
	if !validMutationCSRF(actor, source, r) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}

	id := strings.TrimSpace(r.PathValue("moduleID"))
	if id == "" {
		s.notFound(w, r)
		return
	}
	item, err := s.modules.Get(r.Context(), id)
	if errors.Is(err, modules.ErrModuleNotFound) {
		writeAPIError(w, r, http.StatusNotFound, "module_not_found", "module not found", nil)
		return
	}
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "modules_unavailable", "module registry is unavailable", nil)
		return
	}
	if id != "ai.agent" {
		writeAPIError(w, r, http.StatusConflict, "module_control_unsupported", "runtime control is not supported for this module", nil)
		return
	}

	var request struct {
		Operation string `json:"operation"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	request.Operation = strings.ToLower(strings.TrimSpace(request.Operation))

	previous := item.Status
	switch request.Operation {
	case "enable":
		if err := s.modules.SetStatus(r.Context(), id, "enabled", ""); err != nil {
			writeAPIError(w, r, http.StatusInternalServerError, "module_control_failed", "failed to persist module state", nil)
			return
		}
		s.ai.SetEnabled(true)
	case "disable":
		if err := s.modules.SetStatus(r.Context(), id, "disabled", ""); err != nil {
			writeAPIError(w, r, http.StatusInternalServerError, "module_control_failed", "failed to persist module state", nil)
			return
		}
		s.ai.SetEnabled(false)
	case "restart":
		if err := s.modules.SetStatus(r.Context(), id, "restarting", ""); err != nil {
			writeAPIError(w, r, http.StatusInternalServerError, "module_control_failed", "failed to persist module state", nil)
			return
		}
		s.ai.Restart()
		if err := s.modules.SetStatus(r.Context(), id, "enabled", ""); err != nil {
			_ = s.modules.SetStatus(context.WithoutCancel(r.Context()), id, "error", "failed to persist enabled state after restart")
			writeAPIError(w, r, http.StatusInternalServerError, "module_control_failed", "agent restarted but module state could not be persisted", nil)
			return
		}
	default:
		writeAPIError(w, r, http.StatusBadRequest, "invalid_module_operation", "operation must be enable, disable or restart", nil)
		return
	}

	updated, err := s.modules.Get(r.Context(), id)
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "modules_unavailable", "module registry is unavailable", nil)
		return
	}
	s.security.RecordAudit(
		context.WithoutCancel(r.Context()),
		s.securityRequestContext(r),
		actor,
		"module.runtime.control",
		"module",
		id,
		"success",
		map[string]any{
			"operation":       request.Operation,
			"previous_status": previous,
			"status":          updated.Status,
		},
	)
	if s.realtime != nil {
		s.realtime.Publish(
			"module.runtime.changed",
			map[string]any{"module_id": id, "operation": request.Operation, "status": updated.Status},
			requestIDFromContext(r.Context()),
		)
	}
	writeJSON(w, http.StatusOK, map[string]any{"module": updated})
}
