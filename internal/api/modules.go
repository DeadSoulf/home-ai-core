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

func (s *server) moduleNavigation(
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
	type navigationItem struct {
		ModuleID string                   `json:"module_id"`
		Status   string                   `json:"status"`
		Items    []modules.NavigationItem `json:"items,omitempty"`
	}
	out := make([]navigationItem, 0, len(items))
	for _, item := range items {
		entry := navigationItem{ModuleID: item.Manifest.ID, Status: item.Status}
		if item.Status == "enabled" {
			entry.Items = append(entry.Items, item.Manifest.UI.Navigation...)
		}
		out = append(out, entry)
	}
	writeJSON(w, http.StatusOK, map[string]any{"modules": out})
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

func (s *server) cloudAIModuleTest(
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
	if !s.ai.CloudProviderConfigured() {
		writeAPIError(w, r, http.StatusConflict, "cloud_ai_not_configured", "Cloud AI provider is not configured", nil)
		return
	}
	if err := s.ai.TestCloudProvider(r.Context()); err != nil {
		s.writeAIChatError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
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
	if id != "ai.agent" && id != "ai.cloud" && id != "cameras" {
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
	switch id {
	case "ai.agent":
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
			if cloud, cloudErr := s.modules.Get(r.Context(), "ai.cloud"); cloudErr == nil && cloud.Status == "enabled" {
				_ = s.ai.SetCloudProviderEnabled(false)
				_ = s.modules.SetStatus(r.Context(), "ai.cloud", "disabled", "")
				if s.realtime != nil {
					s.realtime.Publish(
						"module.runtime.changed",
						map[string]any{"module_id": "ai.cloud", "operation": "disable", "status": "disabled", "reason": "dependency_disabled"},
						requestIDFromContext(r.Context()),
					)
				}
			}
		case "restart":
			if err := s.modules.SetStatus(r.Context(), id, "enabled", ""); err != nil {
				writeAPIError(w, r, http.StatusInternalServerError, "module_control_failed", "failed to persist module state", nil)
				return
			}
			s.ai.Restart()
		default:
			writeAPIError(w, r, http.StatusBadRequest, "invalid_module_operation", "operation must be enable, disable or restart", nil)
			return
		}
	case "ai.cloud":
		switch request.Operation {
		case "enable":
			if !s.ai.CloudProviderConfigured() {
				writeAPIError(w, r, http.StatusConflict, "cloud_ai_not_configured", "Cloud AI provider is not configured", nil)
				return
			}

			agent, agentErr := s.modules.Get(r.Context(), "ai.agent")
			if agentErr != nil {
				writeAPIError(w, r, http.StatusInternalServerError, "modules_unavailable", "AI Agent module state is unavailable", nil)
				return
			}
			if agent.Status == "error" {
				writeAPIError(w, r, http.StatusConflict, "ai_agent_error", "AI Agent is in an error state", nil)
				return
			}
			agentAutoEnabled := agent.Status != "enabled"
			if agentAutoEnabled {
				if err := s.modules.SetStatus(r.Context(), "ai.agent", "enabled", ""); err != nil {
					writeAPIError(w, r, http.StatusInternalServerError, "module_control_failed", "failed to enable AI Agent dependency", nil)
					return
				}
				s.ai.SetEnabled(true)
				if s.realtime != nil {
					s.realtime.Publish(
						"module.runtime.changed",
						map[string]any{
							"module_id": "ai.agent",
							"operation": "enable",
							"status":    "enabled",
							"reason":    "dependency_enabled",
						},
						requestIDFromContext(r.Context()),
					)
				}
				s.security.RecordAudit(
					context.WithoutCancel(r.Context()),
					s.securityRequestContext(r),
					actor,
					"module.runtime.control",
					"module",
					"ai.agent",
					"success",
					map[string]any{
						"operation":       "enable",
						"previous_status": agent.Status,
						"status":          "enabled",
						"reason":          "cloud_ai_dependency",
					},
				)
			}

			if err := s.ai.SetCloudProviderEnabled(true); err != nil {
				if agentAutoEnabled {
					_ = s.modules.SetStatus(r.Context(), "ai.agent", agent.Status, "")
					s.ai.SetEnabled(false)
				}
				writeAPIError(w, r, http.StatusServiceUnavailable, "cloud_ai_unavailable", "Cloud AI provider is unavailable", nil)
				return
			}
			if err := s.modules.SetStatus(r.Context(), id, "enabled", ""); err != nil {
				_ = s.ai.SetCloudProviderEnabled(false)
				if agentAutoEnabled {
					_ = s.modules.SetStatus(r.Context(), "ai.agent", agent.Status, "")
					s.ai.SetEnabled(false)
				}
				writeAPIError(w, r, http.StatusInternalServerError, "module_control_failed", "failed to persist module state", nil)
				return
			}
		case "disable":
			_ = s.ai.SetCloudProviderEnabled(false)
			if err := s.modules.SetStatus(r.Context(), id, "disabled", ""); err != nil {
				writeAPIError(w, r, http.StatusInternalServerError, "module_control_failed", "failed to persist module state", nil)
				return
			}
		case "restart":
			if err := s.ai.RestartCloudProvider(); err != nil {
				writeAPIError(w, r, http.StatusServiceUnavailable, "cloud_ai_unavailable", "Cloud AI provider is unavailable", nil)
				return
			}
			if err := s.modules.SetStatus(r.Context(), id, "enabled", ""); err != nil {
				writeAPIError(w, r, http.StatusInternalServerError, "module_control_failed", "failed to persist module state", nil)
				return
			}
		default:
			writeAPIError(w, r, http.StatusBadRequest, "invalid_module_operation", "operation must be enable, disable or restart", nil)
			return
		}
	case "cameras":
		if s.cameras == nil {
			writeAPIError(w, r, http.StatusServiceUnavailable, "camera_sdk_unavailable", "Cameras runtime is unavailable", nil)
			return
		}
		switch request.Operation {
		case "enable":
			if err := s.modules.SetStatus(r.Context(), id, "enabled", ""); err != nil {
				writeAPIError(w, r, http.StatusInternalServerError, "module_control_failed", "failed to persist module state", nil)
				return
			}
			s.cameras.Refresh()
		case "disable":
			if err := s.modules.SetStatus(r.Context(), id, "disabled", ""); err != nil {
				writeAPIError(w, r, http.StatusInternalServerError, "module_control_failed", "failed to persist module state", nil)
				return
			}
		case "restart":
			s.cameras.Refresh()
			if err := s.modules.SetStatus(r.Context(), id, "enabled", ""); err != nil {
				writeAPIError(w, r, http.StatusInternalServerError, "module_control_failed", "failed to persist module state", nil)
				return
			}
		default:
			writeAPIError(w, r, http.StatusBadRequest, "invalid_module_operation", "operation must be enable, disable or restart", nil)
			return
		}

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
