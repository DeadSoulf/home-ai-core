package api

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/modules"
	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/state"
	"github.com/DeadSoulf/home-ai-core/internal/systeminfo"
)

type nvrVideoState interface {
	NVRVideoStorage(context.Context) (state.NVRVideoStorage, bool, error)
	SetNVRVideoStorage(context.Context, state.NVRVideoStorage) error
}

type nvrVideoCandidate struct {
	Device         string `json:"device"`
	FilesystemUUID string `json:"filesystem_uuid"`
	Mountpoint     string `json:"mountpoint"`
	Filesystem     string `json:"filesystem"`
	Label          string `json:"label"`
	SizeBytes      uint64 `json:"size_bytes"`
	FreeBytes      uint64 `json:"free_bytes"`
	Active         bool   `json:"active"`
}

func (s *server) nvrVideoCandidateList(ctx context.Context) ([]nvrVideoCandidate, error) {
	assignments, err := s.state.ListStoragePurposes(ctx)
	if err != nil {
		return nil, err
	}
	nodes := systeminfo.Collect(s.nodeID).BlockTree
	result := make([]nvrVideoCandidate, 0)
	for _, record := range assignments {
		if record.Purpose != state.StoragePurposeNVR || record.FilesystemUUID == "" {
			continue
		}
		node, ok := blockNodeForStoragePurpose(nodes, record)
		if !ok || node.System || (node.Type != "part" && node.Type != "lvm") {
			continue
		}
		fs := strings.ToLower(node.Filesystem)
		// Writable Unix permission semantics are required for NVR UID 1001.
		if fs != "ext4" && fs != "xfs" && fs != "btrfs" {
			continue
		}
		for _, mountpoint := range node.Mountpoints {
			mountpoint = filepath.Clean(mountpoint)
			if !strings.HasPrefix(mountpoint, "/mnt/home-ai-core/") {
				continue
			}
			result = append(result, nvrVideoCandidate{
				Device: node.Path, FilesystemUUID: node.UUID,
				Mountpoint: mountpoint, Filesystem: fs, Label: node.Label,
				SizeBytes: node.SizeBytes, FreeBytes: node.FreeBytes,
			})
			break
		}
	}
	return result, nil
}

func (s *server) nvrVideoStorage(w http.ResponseWriter, r *http.Request, _ security.Actor, _ authSource) {
	if !s.nvrReady(w, r) {
		return
	}
	repo, ok := s.state.(nvrVideoState)
	if !ok {
		writeAPIError(w, r, http.StatusServiceUnavailable, "nvr_storage_unavailable",
			"NVR storage state is unavailable", nil)
		return
	}
	candidates, err := s.nvrVideoCandidateList(r.Context())
	if err != nil {
		writeAPIError(w, r, http.StatusServiceUnavailable, "nvr_storage_unavailable",
			"Unable to inspect NVR video disks", nil)
		return
	}
	active, assigned, err := repo.NVRVideoStorage(r.Context())
	if err != nil {
		writeAPIError(w, r, http.StatusServiceUnavailable, "nvr_storage_unavailable",
			"Unable to inspect NVR video storage state", nil)
		return
	}
	verified := false
	if assigned {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		mount, err := modules.NVRStorageStatus(ctx, active.FilesystemUUID)
		verified = err == nil && mount == filepath.Join(active.Mountpoint, ".home-ai-nvr", "recordings")
	}
	for i := range candidates {
		candidates[i].Active = assigned && candidates[i].FilesystemUUID == active.FilesystemUUID && verified
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"devices": candidates,
		"assigned": assigned, "verified": verified,
		"active": active,
	})
}

func (s *server) nvrVideoStorageActivate(w http.ResponseWriter, r *http.Request, actor security.Actor, source authSource) {
	if !actor.Has("modules.manage") {
		writeAPIError(w, r, http.StatusForbidden, "permission_denied", "Module management permission required", nil)
		return
	}
	if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}
	if !s.nvrReady(w, r) {
		return
	}
	if !s.nvrStorageMu.TryLock() {
		writeAPIError(w, r, http.StatusConflict, "nvr_storage_busy", "NVR storage migration is in progress", nil)
		return
	}
	defer s.nvrStorageMu.Unlock()
	var input struct {
		Device  string `json:"device"`
		Confirm string `json:"confirm"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_nvr_storage", err.Error(), nil)
		return
	}
	if input.Confirm != "MIGRATE_NVR_RECORDINGS" {
		writeAPIError(w, r, http.StatusBadRequest, "nvr_storage_confirmation_required",
			"Confirm NVR recording migration explicitly", nil)
		return
	}
	repo, ok := s.state.(nvrVideoState)
	if !ok {
		writeAPIError(w, r, http.StatusServiceUnavailable, "nvr_storage_unavailable",
			"NVR video storage persistence is unavailable", nil)
		return
	}
	candidates, err := s.nvrVideoCandidateList(r.Context())
	if err != nil {
		writeAPIError(w, r, http.StatusServiceUnavailable, "nvr_storage_unavailable",
			"Unable to inspect NVR disks", nil)
		return
	}
	var candidate *nvrVideoCandidate
	for i := range candidates {
		if candidates[i].Device == input.Device {
			candidate = &candidates[i]
			break
		}
	}
	if candidate == nil {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_nvr_storage",
			"Select a formatted, mounted NVR partition under /mnt/home-ai-core", nil)
		return
	}
	existing, assigned, err := repo.NVRVideoStorage(r.Context())
	if err != nil {
		writeAPIError(w, r, http.StatusServiceUnavailable, "nvr_storage_unavailable",
			"Unable to read NVR storage selection", nil)
		return
	}
	if assigned && existing.FilesystemUUID != candidate.FilesystemUUID {
		writeAPIError(w, r, http.StatusConflict, "nvr_storage_already_active",
			"Move NVR recordings to a different disk only through a separate verified migration", nil)
		return
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(31 * time.Minute))
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()
	mount, err := modules.NVRStorageActivate(ctx, candidate.Mountpoint, candidate.FilesystemUUID)
	if err != nil {
		writeAPIError(w, r, http.StatusBadGateway, "nvr_storage_activation_failed", err.Error(), nil)
		return
	}
	expected := filepath.Join(candidate.Mountpoint, ".home-ai-nvr", "recordings")
	if mount != expected {
		writeAPIError(w, r, http.StatusBadGateway, "nvr_storage_activation_failed",
			"NVR helper returned an unexpected recording mount", nil)
		return
	}
	record := state.NVRVideoStorage{
		DevicePath: candidate.Device, FilesystemUUID: candidate.FilesystemUUID,
		Mountpoint: candidate.Mountpoint,
	}
	if err := repo.SetNVRVideoStorage(ctx, record); err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "nvr_storage_state_incomplete",
			"NVR is running on selected disk but storing the activation record failed; retry to reconcile", nil)
		return
	}
	s.security.RecordAudit(r.Context(), s.securityRequestContext(r), actor,
		"nvr.storage.activate", "module", "nvr.module", "success",
		map[string]any{"device": candidate.Device, "filesystem_uuid": candidate.FilesystemUUID})
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "device": candidate.Device,
		"mountpoint": candidate.Mountpoint,
		"message": "NVR video storage activated; original archive remains as a backup",
	})
}
