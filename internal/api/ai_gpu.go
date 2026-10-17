package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/modules"
	"github.com/DeadSoulf/home-ai-core/internal/security"
)

var aiPCIID = regexp.MustCompile(`^[0-9a-f]{4}:[0-9a-f]{2}:[0-9a-f]{2}\.[0-7]$`)

func aiAgentSupportsGPU(version string) bool {
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return false
	}
	major, errMajor := strconv.Atoi(parts[0])
	minor, errMinor := strconv.Atoi(parts[1])
	if errMajor != nil || errMinor != nil {
		return false
	}
	return major > 0 || (major == 0 && minor >= 2)
}

func (s *server) aiGPUReady(w http.ResponseWriter, r *http.Request) bool {
	if s.modules == nil {
		writeAPIError(w, r, http.StatusServiceUnavailable, "ai_not_running", "AI module unavailable", nil)
		return false
	}
	module, err := s.modules.Get(r.Context(), aiModuleID)
	if err != nil || module.Status != "enabled" {
		writeAPIError(w, r, http.StatusServiceUnavailable, "ai_not_running", "AI module must be enabled", nil)
		return false
	}
	if !aiAgentSupportsGPU(module.Manifest.Version) {
		writeAPIError(w, r, http.StatusConflict, "ai_gpu_image_unsupported",
			"AI Agent 0.2.0 or later is needed for runtime GPU selection", nil)
		return false
	}
	return true
}

// GET does not mutate Docker and exposes only assigned PCI IDs (not Docker
// secrets). A missing helper is reported as unknown, never "CPU verified".
func (s *server) aiGPUAssignment(w http.ResponseWriter, r *http.Request, _ security.Actor, _ authSource) {
	if !s.aiGPUReady(w, r) {
		return
	}
	get := s.aiGPUStatus
	if get == nil {
		get = modules.AssignedAIGPUs
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	pci, err := get(ctx)
	if err != nil {
		writeAPIError(w, r, http.StatusServiceUnavailable, "ai_gpu_runtime_unavailable",
			"Cannot verify current GPU allocation", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"assigned_gpu_pci": pci, "mode": func() string {
		if len(pci) > 0 {
			return "nvidia"
		}
		return "cpu"
	}()})
}

// POST is an explicit opt-in: the UI verifies the selected GPUs and the
// operator confirms that model requests may be briefly interrupted.
// The helper uses a staged candidate and rolls back if health checks fail.
func (s *server) aiGPUApply(w http.ResponseWriter, r *http.Request, actor security.Actor, source authSource) {
	if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}
	if !s.aiGPUReady(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var input struct {
		PCIs         []string `json:"pci_addresses"`
		Confirmation string   `json:"confirmation"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "ai_gpu_invalid_request", "Invalid GPU selection", nil)
		return
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF || input.Confirmation != "ASSIGN_AI_GPU" || input.PCIs == nil ||
		len(input.PCIs) > 8 {
		writeAPIError(w, r, http.StatusBadRequest, "ai_gpu_invalid_request", "Explicit GPU confirmation and PCI array are required", nil)
		return
	}
	seen := map[string]bool{}
	for _, id := range input.PCIs {
		if !aiPCIID.MatchString(id) || seen[id] {
			writeAPIError(w, r, http.StatusBadRequest, "ai_gpu_invalid_request", "Unknown or duplicate PCI address", nil)
			return
		}
		seen[id] = true
	}
	apply := s.aiGPUApplyFunc
	if apply == nil {
		apply = modules.ApplyAIGPUs
	}
	ctx, cancel := context.WithTimeout(r.Context(), 150*time.Second)
	defer cancel()
	assigned, err := apply(ctx, input.PCIs)
	if err != nil {
		// Outcome may be uncertain after a disconnect: never silently
		// resubmit a privileged side effect; ask the UI to inspect status.
		if !errors.Is(err, context.Canceled) && s.logger != nil {
			s.logger.Error("AI GPU reconfiguration failed", "error", err)
		}
		writeAPIError(w, r, http.StatusBadGateway, "ai_gpu_apply_failed",
			"Could not confirm GPU reconfiguration; refresh the assigned GPUs before retrying", nil)
		return
	}
	s.security.RecordAudit(r.Context(), s.securityRequestContext(r), actor,
		"ai.gpu.assign", "module", aiModuleID, "success",
		map[string]any{"gpu_count": len(assigned)})
	writeJSON(w, http.StatusOK, map[string]any{"assigned_gpu_pci": assigned, "mode": func() string {
		if len(assigned) > 0 {
			return "nvidia"
		}
		return "cpu"
	}()})
}
