package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"runtime"
	"strings"

	"github.com/DeadSoulf/home-ai-core/internal/modulecatalog"
	"github.com/DeadSoulf/home-ai-core/internal/modules"
	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/state"
	"github.com/DeadSoulf/home-ai-core/internal/version"
)

type ModuleCatalogService interface {
	Load(context.Context, bool) (modulecatalog.Snapshot, error)
	Module(context.Context, string, bool) (modules.Manifest, modulecatalog.Snapshot, error)
	TokenConfigured() bool
	SaveToken(string) error
	Repository() string
	RegistryCredentials() (string, string)
}

type catalogModuleResponse struct {
	Manifest           modules.Manifest `json:"manifest"`
	Installed          bool             `json:"installed"`
	InstalledVersion   string           `json:"installed_version,omitempty"`
	InstalledStatus    string           `json:"installed_status,omitempty"`
	UpdateAvailable    bool             `json:"update_available"`
	Compatible         bool             `json:"compatible"`
	CompatibilityError string           `json:"compatibility_error,omitempty"`
}

func (s *server) moduleCatalogCollection(
	w http.ResponseWriter,
	r *http.Request,
	_ security.Actor,
	_ authSource,
) {
	if s.moduleCatalog == nil {
		writeAPIError(w, r, http.StatusServiceUnavailable, "module_catalog_unavailable", "official module catalog is not configured", nil)
		return
	}
	refresh := r.URL.Query().Get("refresh") == "1"
	snapshot, err := s.moduleCatalog.Load(r.Context(), refresh)
	if err != nil {
		writeAPIError(w, r, http.StatusServiceUnavailable, "module_catalog_unavailable", err.Error(), nil)
		return
	}
	installed, err := s.modules.List(r.Context())
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "modules_unavailable", "module registry is unavailable", nil)
		return
	}
	capabilities, err := s.modules.Capabilities(r.Context())
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "modules_unavailable", "module capabilities are unavailable", nil)
		return
	}

	installedByID := make(map[string]modules.Registered, len(installed))
	for _, item := range installed {
		installedByID[item.Manifest.ID] = item
	}
	items := make([]catalogModuleResponse, 0, len(snapshot.Catalog.Modules))
	for _, manifest := range snapshot.Catalog.Modules {
		item := catalogModuleResponse{
			Manifest:   manifest,
			Compatible: true,
		}
		if err := modules.CheckCompatibility(manifest, version.Version, runtime.GOARCH, capabilities); err != nil {
			item.Compatible = false
			item.CompatibilityError = err.Error()
		}
		if current, ok := installedByID[manifest.ID]; ok {
			item.Installed = true
			item.InstalledVersion = current.Manifest.Version
			item.InstalledStatus = current.Status
			if comparison, err := modules.CompareVersions(current.Manifest.Version, manifest.Version); err == nil {
				item.UpdateAvailable = comparison < 0
			}
		}
		items = append(items, item)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"catalog": map[string]any{
			"id":           snapshot.Catalog.ID,
			"generated_at": snapshot.Catalog.GeneratedAt,
			"source_sha":   snapshot.SourceSHA,
			"refreshed_at": snapshot.RefreshedAt,
			"from_cache":   snapshot.FromCache,
			"warning":      snapshot.Warning,
			"modules":      items,
		},
	})
}

func (s *server) moduleCatalogAction(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	source authSource,
) {
	if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}
	if s.moduleCatalog == nil {
		writeAPIError(w, r, http.StatusServiceUnavailable, "module_catalog_unavailable", "official module catalog is not configured", nil)
		return
	}

	id := strings.TrimSpace(r.PathValue("moduleID"))
	action := strings.TrimSpace(r.PathValue("action"))
	if id == "" || (action != "install" && action != "update") {
		s.notFound(w, r)
		return
	}

	manifest, _, err := s.moduleCatalog.Module(r.Context(), id, true)
	if err != nil {
		writeAPIError(w, r, http.StatusServiceUnavailable, "module_catalog_unavailable", err.Error(), nil)
		return
	}
	capabilities, err := s.modules.Capabilities(r.Context())
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "modules_unavailable", "module capabilities are unavailable", nil)
		return
	}
	if err := modules.CheckCompatibility(manifest, version.Version, runtime.GOARCH, capabilities); err != nil {
		writeAPIError(w, r, http.StatusConflict, "module_incompatible", err.Error(), nil)
		return
	}

	current, currentErr := s.modules.Get(r.Context(), id)
	switch action {
	case "install":
		if currentErr == nil {
			writeAPIError(w, r, http.StatusConflict, "module_already_installed", "module is already installed", nil)
			return
		}
		if !errors.Is(currentErr, modules.ErrModuleNotFound) {
			writeAPIError(w, r, http.StatusInternalServerError, "modules_unavailable", "module registry is unavailable", nil)
			return
		}
	case "update":
		if errors.Is(currentErr, modules.ErrModuleNotFound) {
			writeAPIError(w, r, http.StatusNotFound, "module_not_found", "module is not installed", nil)
			return
		}
		if currentErr != nil {
			writeAPIError(w, r, http.StatusInternalServerError, "modules_unavailable", "module registry is unavailable", nil)
			return
		}
		comparison, err := modules.CompareVersions(current.Manifest.Version, manifest.Version)
		if err != nil || comparison >= 0 {
			writeAPIError(w, r, http.StatusConflict, "module_update_unavailable", "no newer module version is available", nil)
			return
		}
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
			"module_id":      manifest.ID,
			"manifest_json":  string(raw),
			"catalog_action": action,
		},
	})
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "module_job_failed", "failed to queue module operation", nil)
		return
	}
	s.security.RecordAudit(
		r.Context(),
		s.securityRequestContext(r),
		actor,
		"modules.catalog."+action+".queued",
		"module",
		manifest.ID,
		"success",
		map[string]any{
			"version": manifest.Version,
			"job_id":  job.ID,
		},
	)
	writeJSON(w, http.StatusAccepted, map[string]any{"job": job})
}

func (s *server) moduleCatalogAccessStatus(
	w http.ResponseWriter,
	r *http.Request,
	_ security.Actor,
	_ authSource,
) {
	if s.moduleCatalog == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"access": map[string]any{
				"repository":       modulecatalog.Repository,
				"token_configured": false,
			},
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"access": map[string]any{
			"repository":       s.moduleCatalog.Repository(),
			"token_configured": s.moduleCatalog.TokenConfigured(),
		},
	})
}

func (s *server) moduleCatalogAccessUpdate(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	source authSource,
) {
	if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}
	if s.moduleCatalog == nil {
		writeAPIError(w, r, http.StatusServiceUnavailable, "module_catalog_unavailable", "official module catalog is not configured", nil)
		return
	}

	var input struct {
		Token string `json:"token"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_catalog_access", "invalid catalog access settings", nil)
		return
	}
	if err := s.moduleCatalog.SaveToken(input.Token); err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "catalog_access_failed", err.Error(), nil)
		return
	}

	s.security.RecordAudit(
		r.Context(),
		s.securityRequestContext(r),
		actor,
		"modules.catalog.access.updated",
		"module_catalog",
		modulecatalog.Repository,
		"success",
		map[string]any{"token_configured": strings.TrimSpace(input.Token) != ""},
	)
	writeJSON(w, http.StatusOK, map[string]any{
		"access": map[string]any{
			"repository":       s.moduleCatalog.Repository(),
			"token_configured": s.moduleCatalog.TokenConfigured(),
		},
	})
}
