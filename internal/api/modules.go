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

// Installed registry records, not running-container availability, own sidebar entries.
func (s *server) moduleNavigation(w http.ResponseWriter, r *http.Request, actor security.Actor, _ authSource) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, r, http.MethodGet)
		return
	}
	items, err := s.modules.List(r.Context())
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "modules_unavailable", "module registry is unavailable", nil)
		return
	}
	out := make([]moduleNavigationEntry, 0, len(items))
	for _, item := range items {
		permission := "modules.read"
		if item.Manifest.ID == "ai.agent" || item.Manifest.ID == "frigate.module" {
			permission = "modules.manage"
		}
		allowed := false
		for _, grant := range actor.Permissions {
			if grant == permission {
				allowed = true
				break
			}
		}
		if !allowed {
			continue
		}
		out = append(out, moduleNavigationEntry{ModuleID: item.Manifest.ID, Name: item.Manifest.Name, Status: item.Status, Items: installedModuleNavigation(item.Manifest)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"modules": out})
}

type moduleNavigationEntry struct {
	ModuleID string                   `json:"module_id"`
	Name     string                   `json:"name"`
	Status   string                   `json:"status"`
	Items    []modules.NavigationItem `json:"items"`
}

func installedModuleNavigation(manifest modules.Manifest) []modules.NavigationItem {
	root := "/modules/" + manifest.ID
	seen := map[string]bool{}
	out := make([]modules.NavigationItem, 0, len(manifest.UI.Navigation)+1)
	for _, item := range manifest.UI.Navigation {
		if item.Route != root && !strings.HasPrefix(item.Route, root+"/") {
			continue
		}
		if strings.Contains(item.Route, "..") || seen[item.Route] {
			continue
		}
		if strings.TrimSpace(item.Title) == "" {
			item.Title = manifest.Name
		}
		seen[item.Route] = true
		out = append(out, item)
	}
	if !seen[root] {
		title := manifest.Name
		if title == "" {
			title = manifest.ID
		}
		out = append(out, modules.NavigationItem{ID: manifest.ID, Title: title, Route: root, Order: 50})
	}
	return out
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
	job, err := s.submitModuleJob(r.Context(), state.JobRecord{
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
		writeModuleSubmitError(w, r, err)
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
	case "recover_upgrade":
		if id != "frigate.module" {
			writeAPIError(w, r, http.StatusBadRequest, "invalid_module_operation", "upgrade recovery is restricted to Frigate", nil)
			return
		}
	case "activate_storage":
		if id != "nvr.module" {
			writeAPIError(w, r, http.StatusBadRequest, "invalid_module_operation", "storage activation is restricted to NVR", nil)
			return
		}
	default:
		writeAPIError(w, r, http.StatusBadRequest, "invalid_module_operation", "operation must be enable, disable, restart or NVR storage activation", nil)
		return
	}

	meta := metadataFromContext(r.Context())
	job, err := s.submitModuleJob(r.Context(), state.JobRecord{
		Type:          "module.control",
		ActorType:     actor.Type,
		ActorID:       actor.ID,
		RequestID:     meta.RequestID,
		CorrelationID: meta.CorrelationID,
		Input:         map[string]any{"module_id": id, "operation": input.Operation},
	})
	if err != nil {
		writeModuleSubmitError(w, r, err)
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
	job, err := s.submitModuleJob(r.Context(), state.JobRecord{
		Type:          "module.remove",
		ActorType:     actor.Type,
		ActorID:       actor.ID,
		RequestID:     meta.RequestID,
		CorrelationID: meta.CorrelationID,
		Input:         map[string]any{"module_id": id},
	})
	if err != nil {
		writeModuleSubmitError(w, r, err)
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
