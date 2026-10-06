package api

import (
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/state"
	"github.com/DeadSoulf/home-ai-core/internal/systeminfo"
)

type storagePurposeUsageResponse struct {
	Type     string `json:"type"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	RootPath string `json:"root_path,omitempty"`
}

type storagePurposeResponse struct {
	DevicePath     string                        `json:"device"`
	FilesystemUUID string                        `json:"filesystem_uuid,omitempty"`
	Purpose        string                        `json:"purpose"`
	Present        bool                          `json:"present"`
	Filesystem     string                        `json:"filesystem,omitempty"`
	Label          string                        `json:"label,omitempty"`
	Mountpoints    []string                      `json:"mountpoints"`
	SizeBytes      uint64                        `json:"size_bytes,omitempty"`
	FreeBytes      uint64                        `json:"free_bytes,omitempty"`
	FreeKnown      bool                          `json:"free_known"`
	InUse          bool                          `json:"in_use"`
	UsedBy         []storagePurposeUsageResponse `json:"used_by"`
}

func (s *server) storagePurposes(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	source authSource,
) {
	switch r.Method {
	case http.MethodGet:
		records, err := s.state.ListStoragePurposes(r.Context())
		if err != nil {
			writeAPIError(w, r, http.StatusInternalServerError, "storage_purposes_unavailable", "storage purposes are unavailable", nil)
			return
		}
		nodes := systeminfo.Collect(s.nodeID).BlockTree
		pools, err := s.state.ListNASPools(r.Context())
		if err != nil {
			writeAPIError(w, r, http.StatusInternalServerError, "storage_usage_unavailable", "storage usage is unavailable", nil)
			return
		}
		assignments := make([]storagePurposeResponse, 0, len(records))
		for _, record := range records {
			response := storagePurposeResponseFor(record, nodes)
			response.UsedBy = storageUsageForPurposeRecord(record, nodes, pools)
			response.InUse = len(response.UsedBy) > 0
			assignments = append(assignments, response)
		}
		writeJSON(w, http.StatusOK, map[string]any{"assignments": assignments})

	case http.MethodPost:
		if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
			writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
			return
		}

		var input struct {
			Device  string `json:"device"`
			Purpose string `json:"purpose"`
		}
		if err := decodeJSON(w, r, &input); err != nil {
			writeAPIError(w, r, http.StatusBadRequest, "invalid_storage_purpose", err.Error(), nil)
			return
		}
		input.Device = strings.TrimSpace(input.Device)
		input.Purpose = strings.ToLower(strings.TrimSpace(input.Purpose))
		if input.Device == "" {
			writeAPIError(w, r, http.StatusBadRequest, "invalid_storage_purpose", "storage device is required", nil)
			return
		}

		nodes := systeminfo.Collect(s.nodeID).BlockTree
		node, ok := blockNodeByPath(nodes, input.Device)
		if !ok {
			writeAPIError(w, r, http.StatusNotFound, "storage_device_not_found", "storage device was not found", nil)
			return
		}
		if node.System {
			writeAPIError(w, r, http.StatusBadRequest, "storage_purpose_system_device", "system storage cannot be assigned to an application purpose", nil)
			return
		}
		if node.Type != "part" && node.Type != "lvm" {
			writeAPIError(w, r, http.StatusBadRequest, "storage_purpose_device_type", "only partitions or logical volumes can be assigned", nil)
			return
		}

		purpose := ""
		if input.Purpose != "" && input.Purpose != "none" {
			var err error
			purpose, err = state.NormalizeStoragePurpose(input.Purpose)
			if err != nil {
				writeAPIError(w, r, http.StatusBadRequest, "invalid_storage_purpose", err.Error(), nil)
				return
			}
		}


		pools, err := s.state.ListNASPools(r.Context())
		if err != nil {
			writeAPIError(w, r, http.StatusInternalServerError, "storage_usage_unavailable", "storage usage is unavailable", nil)
			return
		}
		usage := storageUsageForNode(node, pools)
		if len(usage) > 0 && purpose != state.StoragePurposeFiles {
			writeAPIError(
				w,
				r,
				http.StatusConflict,
				"storage_in_use",
				"storage is used by a Home-AI file pool and cannot be reassigned or cleared",
				map[string]any{"used_by": usage},
			)
			return
		}

		if purpose == "" {
			if err := s.state.ClearStoragePurpose(r.Context(), node.Path, node.UUID); err != nil {
				writeAPIError(w, r, http.StatusInternalServerError, "storage_purpose_clear_failed", err.Error(), nil)
				return
			}
			s.security.RecordAudit(
				r.Context(),
				s.securityRequestContext(r),
				actor,
				"storage.purpose.clear",
				"block_device",
				node.Path,
				"success",
				map[string]any{"filesystem_uuid": node.UUID},
			)
			s.realtime.Publish("storage.purpose.changed", map[string]any{
				"device":  node.Path,
				"purpose": "",
			}, requestIDFromContext(r.Context()))
			writeJSON(w, http.StatusOK, map[string]any{
				"assignment": nil,
			})
			return
		}

		record, err := s.state.SetStoragePurpose(
			r.Context(),
			node.Path,
			node.UUID,
			purpose,
			time.Now().UTC(),
		)
		if err != nil {
			writeAPIError(w, r, http.StatusInternalServerError, "storage_purpose_save_failed", err.Error(), nil)
			return
		}

		s.security.RecordAudit(
			r.Context(),
			s.securityRequestContext(r),
			actor,
			"storage.purpose.set",
			"block_device",
			node.Path,
			"success",
			map[string]any{
				"filesystem_uuid": node.UUID,
				"purpose":         purpose,
			},
		)
		s.realtime.Publish("storage.purpose.changed", map[string]any{
			"device":  node.Path,
			"purpose": purpose,
		}, requestIDFromContext(r.Context()))
		writeJSON(w, http.StatusOK, map[string]any{
			"assignment": storagePurposeResponseFor(record, nodes),
		})

	default:
		methodNotAllowed(w, r, http.MethodGet+", "+http.MethodPost)
	}
}

func storagePurposeResponseFor(record state.StoragePurposeRecord, nodes []systeminfo.BlockNode) storagePurposeResponse {
	result := storagePurposeResponse{
		DevicePath:     record.DevicePath,
		FilesystemUUID: record.FilesystemUUID,
		Purpose:        record.Purpose,
		Mountpoints:    []string{},
		UsedBy:         []storagePurposeUsageResponse{},
	}
	node, ok := blockNodeForStoragePurpose(nodes, record)
	if !ok {
		return result
	}
	result.DevicePath = node.Path
	if node.UUID != "" {
		result.FilesystemUUID = node.UUID
	}
	result.Present = true
	result.Filesystem = node.Filesystem
	result.Label = node.Label
	result.Mountpoints = append([]string{}, node.Mountpoints...)
	result.SizeBytes = node.SizeBytes
	result.FreeBytes = node.FreeBytes
	result.FreeKnown = node.FreeKnown
	return result
}

func blockNodeForStoragePurpose(nodes []systeminfo.BlockNode, record state.StoragePurposeRecord) (systeminfo.BlockNode, bool) {
	if record.FilesystemUUID != "" {
		if node, ok := blockNodeByUUID(nodes, record.FilesystemUUID); ok {
			return node, true
		}
	}
	return blockNodeByPath(nodes, record.DevicePath)
}

func blockNodeByPath(nodes []systeminfo.BlockNode, devicePath string) (systeminfo.BlockNode, bool) {
	return findBlockNode(nodes, func(node systeminfo.BlockNode) bool {
		return node.Path == devicePath
	})
}

func blockNodeByUUID(nodes []systeminfo.BlockNode, filesystemUUID string) (systeminfo.BlockNode, bool) {
	return findBlockNode(nodes, func(node systeminfo.BlockNode) bool {
		return node.UUID != "" && node.UUID == filesystemUUID
	})
}

func findBlockNode(nodes []systeminfo.BlockNode, match func(systeminfo.BlockNode) bool) (systeminfo.BlockNode, bool) {
	for _, node := range nodes {
		if match(node) {
			return node, true
		}
		if found, ok := findBlockNode(node.Children, match); ok {
			return found, true
		}
	}
	return systeminfo.BlockNode{}, false
}

func partitionPathsForDisk(nodes []systeminfo.BlockNode, diskPath string) map[string]systeminfo.BlockNode {
	result := map[string]systeminfo.BlockNode{}
	disk, ok := blockNodeByPath(nodes, diskPath)
	if !ok {
		return result
	}
	for _, child := range disk.Children {
		collectPartitionNodes(child, result)
	}
	return result
}

func collectPartitionNodes(node systeminfo.BlockNode, result map[string]systeminfo.BlockNode) {
	if node.Type == "part" && node.Path != "" {
		result[node.Path] = node
	}
	for _, child := range node.Children {
		collectPartitionNodes(child, result)
	}
}

func storageUsageForPurposeRecord(
	record state.StoragePurposeRecord,
	nodes []systeminfo.BlockNode,
	pools []state.NASPoolRecord,
) []storagePurposeUsageResponse {
	if node, ok := blockNodeForStoragePurpose(nodes, record); ok {
		return storageUsageForNode(node, pools)
	}
	paths := map[string]bool{}
	uuids := map[string]bool{}
	if record.DevicePath != "" {
		paths[record.DevicePath] = true
	}
	if record.FilesystemUUID != "" {
		uuids[record.FilesystemUUID] = true
	}
	return storageUsageForIdentity(nil, paths, uuids, pools)
}

func storageUsageForNode(node systeminfo.BlockNode, pools []state.NASPoolRecord) []storagePurposeUsageResponse {
	mountpoints := make([]string, 0)
	paths := map[string]bool{}
	uuids := map[string]bool{}
	collectStorageIdentity(node, &mountpoints, paths, uuids)
	return storageUsageForIdentity(mountpoints, paths, uuids, pools)
}

func collectStorageIdentity(
	node systeminfo.BlockNode,
	mountpoints *[]string,
	paths map[string]bool,
	uuids map[string]bool,
) {
	if node.Path != "" {
		paths[node.Path] = true
	}
	if node.UUID != "" {
		uuids[node.UUID] = true
	}
	for _, mountpoint := range node.Mountpoints {
		mountpoint = strings.TrimSpace(mountpoint)
		if mountpoint != "" {
			*mountpoints = append(*mountpoints, mountpoint)
		}
	}
	for _, child := range node.Children {
		collectStorageIdentity(child, mountpoints, paths, uuids)
	}
}

func storageUsageForMountpoints(mountpoints []string, pools []state.NASPoolRecord) []storagePurposeUsageResponse {
	return storageUsageForIdentity(mountpoints, nil, nil, pools)
}

func storageUsageForIdentity(
	mountpoints []string,
	paths map[string]bool,
	uuids map[string]bool,
	pools []state.NASPoolRecord,
) []storagePurposeUsageResponse {
	if len(pools) == 0 {
		return []storagePurposeUsageResponse{}
	}
	result := make([]storagePurposeUsageResponse, 0)
	for _, pool := range pools {
		matched := false
		switch {
		case pool.StorageFilesystemUUID != "":
			matched = uuids[pool.StorageFilesystemUUID]
		case pool.StorageDevicePath != "":
			matched = paths[pool.StorageDevicePath]
		default:
			for _, mountpoint := range mountpoints {
				if pathUsesStorageMount(pool.RootPath, mountpoint) {
					matched = true
					break
				}
			}
		}
		if !matched {
			continue
		}
		result = append(result, storagePurposeUsageResponse{
			Type:     "file_pool",
			ID:       pool.ID,
			Name:     pool.Name,
			RootPath: pool.RootPath,
		})
	}
	return result
}

func pathUsesStorageMount(rootPath, mountpoint string) bool {
	rootPath = filepath.Clean(strings.TrimSpace(rootPath))
	mountpoint = filepath.Clean(strings.TrimSpace(mountpoint))
	if rootPath == "." || mountpoint == "." || mountpoint == string(filepath.Separator) {
		return rootPath == mountpoint
	}
	relative, err := filepath.Rel(mountpoint, rootPath)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}
