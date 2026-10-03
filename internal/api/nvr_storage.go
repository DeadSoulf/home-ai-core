package api

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	nvrpkg "github.com/DeadSoulf/home-ai-core/internal/nvr"
	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/state"
	"github.com/DeadSoulf/home-ai-core/internal/systeminfo"
)

type nvrStorageState interface {
	SetNVRStorageTarget(
		ctx context.Context,
		devicePath, filesystemUUID, mountpoint string,
		reservePercent int,
		active bool,
		now time.Time,
	) (state.NVRStorageTargetRecord, error)
	ActiveNVRStorageTarget(ctx context.Context) (state.NVRStorageTargetRecord, error)
	NVRArchiveBytes(ctx context.Context, storageTargetID string) (int64, error)
}

type nvrStorageTargetResponse struct {
	ID             string   `json:"id,omitempty"`
	DevicePath     string   `json:"device"`
	FilesystemUUID string   `json:"filesystem_uuid,omitempty"`
	Mountpoint     string   `json:"mountpoint,omitempty"`
	Filesystem     string   `json:"filesystem,omitempty"`
	Label          string   `json:"label,omitempty"`
	SizeBytes      uint64   `json:"size_bytes,omitempty"`
	FreeBytes      uint64   `json:"free_bytes,omitempty"`
	FreeKnown      bool     `json:"free_known"`
	ReservePercent int      `json:"reserve_percent"`
	Active         bool     `json:"active"`
	Ready          bool     `json:"ready"`
	Mountpoints    []string `json:"mountpoints"`
	ArchiveBytes   int64    `json:"archive_bytes,omitempty"`
}

func (s *server) nvrStorage(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	source authSource,
) {
	if !s.nvrEnabled(w, r) {
		return
	}
	if !actor.Has(nvrpkg.PermissionStorageManage) {
		writeAPIError(w, r, http.StatusForbidden, "permission_denied", "NVR storage management permission required", nil)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.nvrStorageGet(w, r)
	case http.MethodPost:
		if !validMutationCSRF(actor, source, r) {
			writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
			return
		}
		s.nvrStorageSet(w, r, actor)
	default:
		methodNotAllowed(w, r, http.MethodGet+", "+http.MethodPost)
	}
}

func (s *server) nvrStorageGet(w http.ResponseWriter, r *http.Request) {
	store, ok := s.state.(nvrStorageState)
	if !ok {
		writeAPIError(w, r, http.StatusServiceUnavailable, "nvr_storage_unavailable", "NVR storage is unavailable", nil)
		return
	}
	records, err := s.state.ListStoragePurposes(r.Context())
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "nvr_storage_unavailable", "NVR storage is unavailable", nil)
		return
	}
	var active state.NVRStorageTargetRecord
	active, activeErr := store.ActiveNVRStorageTarget(r.Context())
	if activeErr != nil && !errors.Is(activeErr, state.ErrNVRStorageTargetNotFound) {
		writeAPIError(w, r, http.StatusInternalServerError, "nvr_storage_unavailable", "NVR storage is unavailable", nil)
		return
	}

	nodes := systeminfo.Collect(s.nodeID).BlockTree
	targets := make([]nvrStorageTargetResponse, 0)
	for _, record := range records {
		if record.Purpose != state.StoragePurposeVideo {
			continue
		}
		response := nvrStorageResponseFor(record, nodes)
		if activeErr == nil && sameStorageIdentity(record.DevicePath, record.FilesystemUUID, active.DevicePath, active.FilesystemUUID) {
			response.ID = active.ID
			response.Active = true
			response.ReservePercent = active.ReservePercent
			if bytes, err := store.NVRArchiveBytes(r.Context(), active.ID); err == nil {
				response.ArchiveBytes = bytes
			}
		}
		targets = append(targets, response)
	}
	writeJSON(w, http.StatusOK, map[string]any{"targets": targets})
}

func (s *server) nvrStorageSet(w http.ResponseWriter, r *http.Request, actor security.Actor) {
	store, ok := s.state.(nvrStorageState)
	if !ok {
		writeAPIError(w, r, http.StatusServiceUnavailable, "nvr_storage_unavailable", "NVR storage is unavailable", nil)
		return
	}
	var input struct {
		Device         string `json:"device"`
		ReservePercent int    `json:"reserve_percent"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_nvr_storage", err.Error(), nil)
		return
	}
	input.Device = strings.TrimSpace(input.Device)
	if input.Device == "" {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_nvr_storage", "video storage device is required", nil)
		return
	}
	if input.ReservePercent == 0 {
		input.ReservePercent = 5
	}
	if input.ReservePercent < 1 || input.ReservePercent > 50 {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_nvr_storage", "reserve percent must be between 1 and 50", nil)
		return
	}

	records, err := s.state.ListStoragePurposes(r.Context())
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "nvr_storage_unavailable", "NVR storage is unavailable", nil)
		return
	}
	nodes := systeminfo.Collect(s.nodeID).BlockTree
	node, ok := blockNodeByPath(nodes, input.Device)
	if !ok {
		writeAPIError(w, r, http.StatusNotFound, "nvr_storage_not_found", "video storage device was not found", nil)
		return
	}
	if node.System || (node.Type != "part" && node.Type != "lvm") {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_nvr_storage", "NVR storage must be a non-system partition or logical volume", nil)
		return
	}

	var purpose state.StoragePurposeRecord
	foundPurpose := false
	for _, record := range records {
		if record.Purpose != state.StoragePurposeVideo {
			continue
		}
		if sameStorageIdentity(record.DevicePath, record.FilesystemUUID, node.Path, node.UUID) {
			purpose = record
			foundPurpose = true
			break
		}
	}
	if !foundPurpose {
		writeAPIError(w, r, http.StatusConflict, "nvr_storage_not_video", "storage must be assigned to purpose video before it can be used by NVR", nil)
		return
	}

	mountpoint := preferredNVRMountpoint(node.Mountpoints)
	if mountpoint == "" {
		writeAPIError(w, r, http.StatusConflict, "nvr_storage_not_mounted", "video storage must be mounted before it can be used by NVR", nil)
		return
	}

	target, err := store.SetNVRStorageTarget(
		r.Context(),
		node.Path,
		node.UUID,
		mountpoint,
		input.ReservePercent,
		true,
		time.Now().UTC(),
	)
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "nvr_storage_save_failed", "NVR storage target could not be saved", nil)
		return
	}
	if s.nvr != nil {
		_ = s.nvr.RefreshRecordings(r.Context())
	}

	s.security.RecordAudit(
		r.Context(),
		s.securityRequestContext(r),
		actor,
		"nvr.storage.select",
		"block_device",
		node.Path,
		"success",
		map[string]any{
			"filesystem_uuid": purpose.FilesystemUUID,
			"reserve_percent": input.ReservePercent,
		},
	)
	s.realtime.Publish("nvr.storage.changed", map[string]any{
		"device":          node.Path,
		"reserve_percent": input.ReservePercent,
	}, requestIDFromContext(r.Context()))

	response := nvrStorageResponseFor(purpose, nodes)
	response.ID = target.ID
	response.Active = true
	response.ReservePercent = target.ReservePercent
	if bytes, err := store.NVRArchiveBytes(r.Context(), target.ID); err == nil {
		response.ArchiveBytes = bytes
	}
	writeJSON(w, http.StatusOK, map[string]any{"target": response})
}

func nvrStorageResponseFor(record state.StoragePurposeRecord, nodes []systeminfo.BlockNode) nvrStorageTargetResponse {
	response := nvrStorageTargetResponse{
		DevicePath:     record.DevicePath,
		FilesystemUUID: record.FilesystemUUID,
		ReservePercent: 5,
		Mountpoints:    []string{},
	}
	node, ok := blockNodeForStoragePurpose(nodes, record)
	if !ok {
		return response
	}
	response.DevicePath = node.Path
	if node.UUID != "" {
		response.FilesystemUUID = node.UUID
	}
	response.Filesystem = node.Filesystem
	response.Label = node.Label
	response.SizeBytes = node.SizeBytes
	response.FreeBytes = node.FreeBytes
	response.FreeKnown = node.FreeKnown
	response.Mountpoints = append([]string{}, node.Mountpoints...)
	response.Mountpoint = preferredNVRMountpoint(node.Mountpoints)
	response.Ready = response.Mountpoint != ""
	return response
}

func preferredNVRMountpoint(values []string) string {
	for _, value := range values {
		value = filepath.Clean(strings.TrimSpace(value))
		if value == "." || value == "" || value == string(filepath.Separator) || !filepath.IsAbs(value) {
			continue
		}
		return value
	}
	return ""
}

func sameStorageIdentity(deviceA, uuidA, deviceB, uuidB string) bool {
	uuidA = strings.TrimSpace(uuidA)
	uuidB = strings.TrimSpace(uuidB)
	if uuidA != "" && uuidB != "" {
		return uuidA == uuidB
	}
	return strings.TrimSpace(deviceA) != "" && strings.TrimSpace(deviceA) == strings.TrimSpace(deviceB)
}

func (s *server) activeNVRStorageUsage(
	ctx context.Context,
	node systeminfo.BlockNode,
) (storagePurposeUsageResponse, bool) {
	store, ok := s.state.(nvrStorageState)
	if !ok {
		return storagePurposeUsageResponse{}, false
	}
	target, err := store.ActiveNVRStorageTarget(ctx)
	if err != nil {
		return storagePurposeUsageResponse{}, false
	}
	mountpoints := make([]string, 0)
	paths := map[string]bool{}
	uuids := map[string]bool{}
	collectStorageIdentity(node, &mountpoints, paths, uuids)

	matched := false
	if strings.TrimSpace(target.FilesystemUUID) != "" {
		matched = uuids[target.FilesystemUUID]
	} else {
		matched = paths[target.DevicePath]
	}
	if !matched {
		return storagePurposeUsageResponse{}, false
	}
	return storagePurposeUsageResponse{
		Type:     "nvr_archive",
		ID:       target.ID,
		Name:     "NVR archive",
		RootPath: target.Mountpoint,
	}, true
}
