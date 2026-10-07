package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/DeadSoulf/home-ai-core/internal/modules"
	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/state"
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

func (s *server) moduleInstall(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	source authSource,
) {
	if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}
	manifest, err := modules.DecodeManifest(r.Body)
	if err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_module_manifest", err.Error(), nil)
		return
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "module_job_failed", "failed to prepare module job", nil)
		return
	}
	meta := metadataFromContext(r.Context())
	job, err := s.jobs.Submit(r.Context(), state.JobRecord{
		Type:          "module.install",
		ActorType:     actor.Type,
		ActorID:       actor.ID,
		RequestID:     meta.RequestID,
		CorrelationID: meta.CorrelationID,
		Input: map[string]any{
			"module_id":     manifest.ID,
			"manifest_json": string(raw),
		},
	})
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "module_job_failed", "failed to queue module installation", nil)
		return
	}
	s.security.RecordAudit(
		r.Context(),
		s.securityRequestContext(r),
		actor,
		"modules.install.queued",
		"module",
		manifest.ID,
		"success",
		map[string]any{"version": manifest.Version, "job_id": job.ID},
	)
	writeJSON(w, http.StatusAccepted, map[string]any{"job": job})
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
	if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}

	id := strings.TrimSpace(r.PathValue("moduleID"))
	if id == "" {
		s.notFound(w, r)
		return
	}
	if _, err := s.modules.Get(r.Context(), id); errors.Is(err, modules.ErrModuleNotFound) {
		writeAPIError(w, r, http.StatusNotFound, "module_not_found", "module not found", nil)
		return
	} else if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "modules_unavailable", "module registry is unavailable", nil)
		return
	}

	var input struct {
		Operation string `json:"operation"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_module_operation", "invalid module operation", nil)
		return
	}
	input.Operation = strings.TrimSpace(input.Operation)
	switch input.Operation {
	case "enable", "disable", "restart":
	default:
		writeAPIError(w, r, http.StatusBadRequest, "invalid_module_operation", "operation must be enable, disable or restart", nil)
		return
	}

	meta := metadataFromContext(r.Context())
	job, err := s.jobs.Submit(r.Context(), state.JobRecord{
		Type:          "module.control",
		ActorType:     actor.Type,
		ActorID:       actor.ID,
		RequestID:     meta.RequestID,
		CorrelationID: meta.CorrelationID,
		Input: map[string]any{"module_id": id, "operation": input.Operation},
	})
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "module_job_failed", "failed to queue module operation", nil)
		return
	}
	s.security.RecordAudit(
		r.Context(),
		s.securityRequestContext(r),
		actor,
		"modules."+input.Operation+".queued",
		"module",
		id,
		"success",
		map[string]any{"job_id": job.ID},
	)
	writeJSON(w, http.StatusAccepted, map[string]any{"job": job})
}

func (s *server) moduleRemove(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	source authSource,
) {
	if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}
	id := strings.TrimSpace(r.PathValue("moduleID"))
	if id == "" {
		s.notFound(w, r)
		return
	}
	if _, err := s.modules.Get(r.Context(), id); errors.Is(err, modules.ErrModuleNotFound) {
		writeAPIError(w, r, http.StatusNotFound, "module_not_found", "module not found", nil)
		return
	} else if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "modules_unavailable", "module registry is unavailable", nil)
		return
	}

	meta := metadataFromContext(r.Context())
	job, err := s.jobs.Submit(r.Context(), state.JobRecord{
		Type:          "module.remove",
		ActorType:     actor.Type,
		ActorID:       actor.ID,
		RequestID:     meta.RequestID,
		CorrelationID: meta.CorrelationID,
		Input:         map[string]any{"module_id": id},
	})
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "module_job_failed", "failed to queue module removal", nil)
		return
	}
	s.security.RecordAudit(
		r.Context(),
		s.securityRequestContext(r),
		actor,
		"modules.remove.queued",
		"module",
		id,
		"success",
		map[string]any{"job_id": job.ID, "data_preserved": true},
	)
	writeJSON(w, http.StatusAccepted, map[string]any{"job": job})
}
