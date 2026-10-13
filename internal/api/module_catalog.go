package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/modules"
	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/state"
)

const officialModuleCatalogAPI = "https://api.github.com/repos/DeadSoulf/home-ai-core_modules/contents/catalog.json?ref=main"

type moduleCatalogDocument struct {
	SchemaVersion int                `json:"schema_version"`
	ID            string             `json:"id"`
	GeneratedAt   string             `json:"generated_at"`
	Modules       []modules.Manifest `json:"modules"`
}

type githubContentResponse struct {
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
}

func loadOfficialModuleCatalog(r *http.Request) (moduleCatalogDocument, error) {
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, officialModuleCatalogAPI, nil)
	if err != nil {
		return moduleCatalogDocument{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "home-ai-core")
	if token := strings.TrimSpace(os.Getenv("HOME_AI_MODULE_CATALOG_TOKEN")); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return moduleCatalogDocument{}, fmt.Errorf("request official module catalog: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if resp.StatusCode == http.StatusNotFound && strings.TrimSpace(os.Getenv("HOME_AI_MODULE_CATALOG_TOKEN")) == "" {
			return moduleCatalogDocument{}, errors.New("official module catalog is private; HOME_AI_MODULE_CATALOG_TOKEN is not configured")
		}
		return moduleCatalogDocument{}, fmt.Errorf("GitHub catalog request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload githubContentResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&payload); err != nil {
		return moduleCatalogDocument{}, fmt.Errorf("decode GitHub catalog response: %w", err)
	}
	if payload.Encoding != "base64" {
		return moduleCatalogDocument{}, fmt.Errorf("unsupported GitHub catalog encoding %q", payload.Encoding)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(payload.Content, "\n", ""))
	if err != nil {
		return moduleCatalogDocument{}, fmt.Errorf("decode official module catalog: %w", err)
	}
	var catalog moduleCatalogDocument
	if err := json.Unmarshal(raw, &catalog); err != nil {
		return moduleCatalogDocument{}, fmt.Errorf("parse official module catalog: %w", err)
	}
	if catalog.SchemaVersion != 1 {
		return moduleCatalogDocument{}, fmt.Errorf("unsupported module catalog schema version %d", catalog.SchemaVersion)
	}
	for _, manifest := range catalog.Modules {
		if err := modules.ValidateManifest(manifest); err != nil {
			return moduleCatalogDocument{}, fmt.Errorf("invalid catalog module %q: %w", manifest.ID, err)
		}
	}
	return catalog, nil
}

func (s *server) moduleCatalog(
	w http.ResponseWriter,
	r *http.Request,
	_ security.Actor,
	_ authSource,
) {
	catalog, err := loadOfficialModuleCatalog(r)
	if err != nil {
		writeAPIError(w, r, http.StatusServiceUnavailable, "module_catalog_unavailable", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"catalog": map[string]any{
			"id":           catalog.ID,
			"generated_at": catalog.GeneratedAt,
			"modules":      catalog.Modules,
		},
	})
}

func (s *server) moduleCatalogInstall(
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
	catalog, err := loadOfficialModuleCatalog(r)
	if err != nil {
		writeAPIError(w, r, http.StatusServiceUnavailable, "module_catalog_unavailable", err.Error(), nil)
		return
	}
	var manifest *modules.Manifest
	for i := range catalog.Modules {
		if catalog.Modules[i].ID == id {
			manifest = &catalog.Modules[i]
			break
		}
	}
	if manifest == nil {
		writeAPIError(w, r, http.StatusNotFound, "module_not_in_catalog", "module is not present in the official catalog", nil)
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
		"modules.catalog.install.queued",
		"module",
		manifest.ID,
		"success",
		map[string]any{"version": manifest.Version, "job_id": job.ID, "catalog": catalog.ID},
	)
	writeJSON(w, http.StatusAccepted, map[string]any{"job": job})
}
