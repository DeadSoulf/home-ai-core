package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/frigate"
	"github.com/DeadSoulf/home-ai-core/internal/modules"
	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/state"
)

type frigateUpstreamClient interface {
	Discover(context.Context, string) (frigate.Discovery, error)
	Resolve(context.Context, string, string) (frigate.Candidate, error)
}
type frigateSourceStatus struct {
	frigate.Discovery
	Repository string `json:"repository"`
	Stale      bool   `json:"stale"`
	Error      string `json:"error,omitempty"`
}
type frigateCatalogCache struct {
	mu          sync.Mutex
	lastAttempt time.Time
	value       frigateSourceStatus
}

func (s *server) frigateClient() frigateUpstreamClient {
	if s.frigateUpstream != nil {
		return s.frigateUpstream
	}
	return frigate.NewClient(nil)
}
func (s *server) frigateReleaseStatus(ctx context.Context, refresh bool) frigateSourceStatus {
	cache := &s.frigateCatalog
	cache.mu.Lock()
	defer cache.mu.Unlock()
	age := time.Since(cache.lastAttempt)
	if !cache.lastAttempt.IsZero() && (age < 30*time.Second || (!refresh && age < 5*time.Minute)) {
		return cache.value
	}
	if err := ctx.Err(); err != nil {
		value := cache.value
		value.Stale = true
		value.Error = "Release discovery was cancelled"
		return value
	}
	cache.lastAttempt = time.Now()
	requestCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	discovery, err := s.frigateClient().Discover(requestCtx, runtime.GOARCH)
	if err != nil {
		// Keep last-known-good information, but never use it to authorize mutation.
		cache.value.Stale = true
		cache.value.Error = "Official Frigate release discovery is unavailable; no update will be installed automatically"
		cache.value.Repository = frigate.Repository
		if cache.value.SupportedRange == "" {
			cache.value.SupportedRange = frigate.SupportedVersions
		}
	} else {
		cache.value = frigateSourceStatus{Discovery: discovery, Repository: frigate.Repository}
	}
	return cache.value
}
func (s *server) frigateReleases(w http.ResponseWriter, r *http.Request, _ security.Actor, _ authSource) {
	writeJSON(w, http.StatusOK, map[string]any{"upstream": s.frigateReleaseStatus(r.Context(), r.URL.Query().Get("refresh") == "1")})
}
func (s *server) loadModuleCatalog(r *http.Request) (moduleCatalogDocument, error) {
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	catalog, err := loadOfficialModuleCatalog(r)
	if err != nil {
		return catalog, err
	}
	return s.expandFrigateCatalog(r.Context(), catalog, r.URL.Query().Get("refresh") == "1"), nil
}

type frigateApproval struct {
	Version string `json:"expected_version"`
	Image   string `json:"expected_image"`
}

func decodeFrigateApproval(w http.ResponseWriter, r *http.Request) (frigateApproval, error) {
	var approval frigateApproval
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&approval); err != nil {
		return approval, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return approval, errors.New("one approval object is required")
	}
	if !frigate.CompatibleVersion(approval.Version) || !frigate.ValidImage(approval.Image) {
		return approval, errors.New("approve a supported version and its exact official immutable image")
	}
	return approval, nil
}

// A manual action carries the exact version+digest displayed in the confirmation
// dialog. A release or tag changing while that dialog is open cannot silently
// substitute another image. The job independently revalidates this provenance.
func (s *server) queueFrigateMutation(w http.ResponseWriter, r *http.Request, actor security.Actor, source authSource, upgrade bool) {
	if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}
	approval, err := decodeFrigateApproval(w, r)
	if err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "frigate_approval_required", "Review and confirm the exact Frigate version and digest before installation or update", nil)
		return
	}
	if runtime.GOOS != "linux" {
		writeAPIError(w, r, http.StatusConflict, "frigate_host_incompatible", "Frigate requires a Linux host", nil)
		return
	}
	if upgrade {
		installed, err := s.modules.Get(r.Context(), frigate.ModuleID)
		if err != nil {
			writeAPIError(w, r, http.StatusConflict, "frigate_not_installed", "Frigate must be installed before an upgrade", nil)
			return
		}
		if installed.Status != "enabled" {
			writeAPIError(w, r, http.StatusConflict, "module_not_running", "Frigate must be running for a safe upgrade", nil)
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	candidate, err := s.frigateClient().Resolve(ctx, approval.Version, runtime.GOARCH)
	if err != nil {
		writeAPIError(w, r, http.StatusServiceUnavailable, "frigate_verification_failed", "Could not verify the approved official release and image; no operation was queued", nil)
		return
	}
	if candidate.Image != approval.Image || candidate.Version != approval.Version || candidate.Architecture != runtime.GOARCH {
		writeAPIError(w, r, http.StatusConflict, "frigate_candidate_changed", "The official image changed after approval; refresh releases and review its digest again", nil)
		return
	}
	manifest := modules.FrigateManifest(candidate)
	if err := modules.ValidateManifest(manifest); err != nil {
		writeAPIError(w, r, http.StatusConflict, "frigate_incompatible", "The verified image is not compatible with the integration contract", nil)
		return
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "module_job_failed", "Could not encode the verified Frigate manifest", nil)
		return
	}
	operation := "install"
	if upgrade {
		operation = "upgrade"
	}
	meta := metadataFromContext(r.Context())
	job, err := s.submitModuleJob(r.Context(), state.JobRecord{
		Type: "module." + operation, ActorType: actor.Type, ActorID: actor.ID, RequestID: meta.RequestID, CorrelationID: meta.CorrelationID,
		Input: map[string]any{"module_id": frigate.ModuleID, "manifest_json": string(raw)},
	})
	if err != nil {
		writeModuleSubmitError(w, r, err)
		return
	}
	s.security.RecordAudit(r.Context(), s.securityRequestContext(r), actor, "modules.frigate."+operation+".queued", "module", frigate.ModuleID, "success",
		map[string]any{"version": candidate.Version, "image": candidate.Image, "index_digest": candidate.IndexDigest, "source": frigate.Repository, "job_id": job.ID})
	writeJSON(w, http.StatusAccepted, map[string]any{"job": job})
}

// A catalog declaration is opt-in metadata, not an arbitrary upstream URL.
type moduleUpstreamSource struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Provider        string `json:"provider"`
	Repository      string `json:"repository"`
	ImageRepository string `json:"image_repository"`
}

func frigateSourceEnabled(sources []moduleUpstreamSource) (bool, error) {
	found := false
	for _, source := range sources {
		if source.ID != frigate.ModuleID {
			continue
		}
		if found || source.Provider != "github-ghcr-official" || source.Repository != frigate.Repository || source.ImageRepository != frigate.ImageRepository {
			return false, errors.New("invalid or duplicated official Frigate source declaration")
		}
		found = true
	}
	return found, nil
}
func (s *server) expandFrigateCatalog(ctx context.Context, catalog moduleCatalogDocument, refresh bool) moduleCatalogDocument {
	// Never display a stale/static Frigate digest as an approved upstream result.
	filtered := make([]modules.Manifest, 0, len(catalog.Modules)+1)
	for _, manifest := range catalog.Modules {
		if manifest.ID != frigate.ModuleID {
			filtered = append(filtered, manifest)
		}
	}
	catalog.Modules = filtered
	enabled, err := frigateSourceEnabled(catalog.Upstreams)
	if err != nil {
		catalog.FrigateUpstream = &frigateSourceStatus{Repository: frigate.Repository, Stale: true, Error: "The official Frigate source declaration is invalid"}
		return catalog
	}
	if !enabled {
		return catalog
	}
	upstream := s.frigateReleaseStatus(ctx, refresh)
	catalog.FrigateUpstream = &upstream
	if upstream.Candidate != nil {
		catalog.Modules = append(catalog.Modules, modules.FrigateManifest(*upstream.Candidate))
	}
	return catalog
}
