package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/modules"
	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/state"
)

const (
	officialModuleCatalogAPI = "https://api.github.com/repos/DeadSoulf/home-ai-core_modules/contents/catalog.json?ref=main"
	// Private SDK611 overrides are only read when the host explicitly enables
	// this channel. Public NVR users retain their downloadable stable image.
	privateNVRManifestAPI = "https://api.github.com/repos/DeadSoulf/home-ai-bridge/contents/modules/nvr.sdk611.manifest.json?ref=main"
	privateNVRImagePrefix = "ghcr.io/deadsoulf/home-ai-core_dvr-sdk611@sha256:"
)

type moduleCatalogDocument struct {
	SchemaVersion       int                    `json:"schema_version"`
	ID                  string                 `json:"id"`
	GeneratedAt         string                 `json:"generated_at"`
	Modules             []modules.Manifest     `json:"modules"`
	Upstreams           []moduleUpstreamSource `json:"upstreams,omitempty"`
	PrivateNVRStatus    string                 `json:"-"`
	FrigateUpstream     *frigateSourceStatus   `json:"-"`
	HomeAssistantStatus string                 `json:"-"`
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
	// Never replace the public NVR image for other users. Only a host with
	// explicit SDK611 opt-in *and* authenticated private GitHub access sees
	// the private candidate in its own Core UI.
	if strings.TrimSpace(os.Getenv("HOME_AI_MODULE_PRIVATE_SDK611")) == "1" {
		token := strings.TrimSpace(os.Getenv("HOME_AI_MODULE_CATALOG_TOKEN"))
		if token == "" {
			catalog.PrivateNVRStatus = "missing_token"
		} else {
			manifest, err := fetchPrivateNVRManifest(r.Context(), &http.Client{Timeout: 5 * time.Second}, token, privateNVRManifestAPI)
			if err != nil {
				catalog.PrivateNVRStatus = "unavailable"
			} else {
				catalog.PrivateNVRStatus = mergePrivateNVRCandidate(&catalog, manifest)
			}
		}
	}
	refreshHomeAssistantCandidate(r.Context(), &catalog, nil)
	return catalog, nil
}

// fetchPrivateNVRManifest accepts only an authenticated, constant GitHub
// Contents endpoint in production. Never forward caller-controlled URLs from
// the browser to the privileged module helper or public catalog.
func fetchPrivateNVRManifest(ctx context.Context, client *http.Client, token, url string) (modules.Manifest, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return modules.Manifest{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "home-ai-core")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		return modules.Manifest{}, fmt.Errorf("read private NVR update metadata: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// Do not report response bodies: they may contain authentication or
		// private repository information, and never fall back to an unsafe image.
		return modules.Manifest{}, fmt.Errorf("private NVR metadata unavailable (HTTP %d)", resp.StatusCode)
	}
	var payload githubContentResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 256<<10)).Decode(&payload); err != nil {
		return modules.Manifest{}, fmt.Errorf("decode private NVR metadata: %w", err)
	}
	if payload.Encoding != "base64" {
		return modules.Manifest{}, errors.New("private NVR metadata has unsupported encoding")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(payload.Content, "\n", ""))
	if err != nil || len(raw) > 64<<10 {
		return modules.Manifest{}, errors.New("private NVR manifest is invalid or oversized")
	}
	var candidate modules.Manifest
	if err := json.Unmarshal(raw, &candidate); err != nil {
		return modules.Manifest{}, errors.New("private NVR manifest is invalid")
	}
	if err := modules.ValidateManifest(candidate); err != nil {
		return modules.Manifest{}, fmt.Errorf("private NVR manifest validation failed: %w", err)
	}
	if candidate.ID != "nvr.module" || !strings.HasPrefix(candidate.Runtime.Docker.Image, privateNVRImagePrefix) {
		return modules.Manifest{}, errors.New("private NVR manifest has an untrusted module ID or image repository")
	}
	return candidate, nil
}

func newerNVRSemver(candidate, stable string) bool {
	parse := func(value string) ([3]uint64, bool) {
		var parts [3]uint64
		tokens := strings.Split(strings.TrimPrefix(value, "v"), ".")
		if len(tokens) != 3 {
			return parts, false
		}
		for i, token := range tokens {
			if token == "" || strings.Trim(token, "0123456789") != "" {
				return parts, false
			}
			num, err := strconv.ParseUint(token, 10, 64)
			if err != nil {
				return parts, false
			}
			parts[i] = num
		}
		return parts, true
	}
	c, okC := parse(candidate)
	s, okS := parse(stable)
	if !okC || !okS {
		return false
	}
	for i := range c {
		if c[i] != s[i] {
			return c[i] > s[i]
		}
	}
	return false
}

func mergePrivateNVRCandidate(catalog *moduleCatalogDocument, private modules.Manifest) string {
	for i := range catalog.Modules {
		if catalog.Modules[i].ID != private.ID {
			continue
		}
		if !newerNVRSemver(private.Version, catalog.Modules[i].Version) {
			return "up_to_date"
		}
		catalog.Modules[i] = private
		return "available"
	}
	return "unavailable"
}

func (s *server) moduleCatalog(
	w http.ResponseWriter,
	r *http.Request,
	_ security.Actor,
	_ authSource,
) {
	catalog, err := s.loadModuleCatalog(r)
	if err != nil {
		writeAPIError(w, r, http.StatusServiceUnavailable, "module_catalog_unavailable", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"catalog": map[string]any{
			"id":                   catalog.ID,
			"generated_at":         catalog.GeneratedAt,
			"modules":              catalog.Modules,
			"private_nvr_status":   catalog.PrivateNVRStatus,
			"frigate_upstream":     catalog.FrigateUpstream,
			"homeassistant_status": catalog.HomeAssistantStatus,
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
	if id == "frigate.module" {
		s.queueFrigateMutation(w, r, actor, source, false)
		return
	}
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
		"modules.catalog.install.queued",
		"module",
		manifest.ID,
		"success",
		map[string]any{"version": manifest.Version, "job_id": job.ID, "catalog": catalog.ID},
	)
	writeJSON(w, http.StatusAccepted, map[string]any{"job": job})
}

// moduleCatalogUpgrade never accepts an arbitrary image from a browser.
// The official catalog is reloaded on each request and the registry validates
// both the installed and candidate immutable manifests before switching Docker.
func (s *server) moduleCatalogUpgrade(w http.ResponseWriter, r *http.Request, actor security.Actor, source authSource) {
	if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}
	id := strings.TrimSpace(r.PathValue("moduleID"))
	if id == "frigate.module" {
		s.queueFrigateMutation(w, r, actor, source, true)
		return
	}
	if id == "" {
		s.notFound(w, r)
		return
	}
	installed, err := s.modules.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, modules.ErrModuleNotFound) {
			writeAPIError(w, r, http.StatusNotFound, "module_not_found", "module not found", nil)
		} else {
			writeAPIError(w, r, http.StatusServiceUnavailable, "modules_unavailable", "module registry is unavailable", nil)
		}
		return
	}
	if installed.Status != "enabled" {
		writeAPIError(w, r, http.StatusConflict, "module_not_running", "module must be running to upgrade safely", nil)
		return
	}
	catalog, err := loadOfficialModuleCatalog(r)
	if err != nil {
		writeAPIError(w, r, http.StatusServiceUnavailable, "module_catalog_unavailable", err.Error(), nil)
		return
	}
	var candidate *modules.Manifest
	for i := range catalog.Modules {
		if catalog.Modules[i].ID == id {
			candidate = &catalog.Modules[i]
			break
		}
	}
	if candidate == nil {
		writeAPIError(w, r, http.StatusNotFound, "module_not_in_catalog", "module is not present in the official catalog", nil)
		return
	}
	raw, err := json.Marshal(candidate)
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "module_job_failed", "failed to prepare module upgrade", nil)
		return
	}
	meta := metadataFromContext(r.Context())
	job, err := s.submitModuleJob(r.Context(), state.JobRecord{
		Type: "module.upgrade", ActorType: actor.Type, ActorID: actor.ID,
		RequestID: meta.RequestID, CorrelationID: meta.CorrelationID,
		Input: map[string]any{"module_id": id, "manifest_json": string(raw)},
	})
	if err != nil {
		writeModuleSubmitError(w, r, err)
		return
	}
	s.security.RecordAudit(r.Context(), s.securityRequestContext(r), actor,
		"modules.catalog.upgrade.queued", "module", id, "success",
		map[string]any{"version": candidate.Version, "job_id": job.ID, "catalog": catalog.ID})
	writeJSON(w, http.StatusAccepted, map[string]any{"job": job})
}
