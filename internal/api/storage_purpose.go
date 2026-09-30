package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/state"
	"github.com/DeadSoulf/home-ai-core/internal/systeminfo"
)

type storagePurposeResponse struct {
	DevicePath     string   `json:"device"`
	FilesystemUUID string   `json:"filesystem_uuid,omitempty"`
	Purpose        string   `json:"purpose"`
	Present        bool     `json:"present"`
	Filesystem     string   `json:"filesystem,omitempty"`
	Label          string   `json:"label,omitempty"`
	Mountpoints    []string `json:"mountpoints"`
	SizeBytes      uint64   `json:"size_bytes,omitempty"`
	FreeBytes      uint64   `json:"free_bytes,omitempty"`
	FreeKnown      bool     `json:"free_known"`
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
		assignments := make([]storagePurposeResponse, 0, len(records))
		for _, record := range records {
			assignments = append(assignments, storagePurposeResponseFor(record, nodes))
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

		if input.Purpose == "" || input.Purpose == "none" {
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

		purpose, err := state.NormalizeStoragePurpose(input.Purpose)
		if err != nil {
			writeAPIError(w, r, http.StatusBadRequest, "invalid_storage_purpose", err.Error(), nil)
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
	result.Mountpoints = append([]string(nil), node.Mountpoints...)
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
