package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/state"
	"github.com/DeadSoulf/home-ai-core/internal/storage"
	"github.com/DeadSoulf/home-ai-core/internal/systeminfo"
)

func (s *server) storageOperation(
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

	var input struct {
		Operation  string `json:"operation"`
		Device     string `json:"device"`
		Mountpoint string `json:"mountpoint,omitempty"`
		Filesystem string `json:"filesystem,omitempty"`
		Label      string `json:"label,omitempty"`
		Confirm    string `json:"confirm,omitempty"`
		SizeMiB    uint64 `json:"size_mib,omitempty"`
		Purpose    string `json:"purpose,omitempty"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_storage_request", "invalid storage request", nil)
		return
	}

	input.Operation = strings.TrimSpace(input.Operation)
	input.Device = strings.TrimSpace(input.Device)
	input.Purpose = strings.ToLower(strings.TrimSpace(input.Purpose))
	if input.Operation == "" || input.Device == "" {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_storage_request", "storage operation and device are required", nil)
		return
	}

	if input.Purpose != "" && input.Purpose != "none" {
		if input.Operation != "partition.create" {
			writeAPIError(w, r, http.StatusBadRequest, "invalid_storage_request", "storage purpose can only be supplied while creating a partition", nil)
			return
		}
		if _, err := state.NormalizeStoragePurpose(input.Purpose); err != nil {
			writeAPIError(w, r, http.StatusBadRequest, "invalid_storage_purpose", err.Error(), nil)
			return
		}
	}

	var beforeTree []systeminfo.BlockNode
	var beforePurposes []state.StoragePurposeRecord
	switch input.Operation {
	case "unmount", "partition.create", "partition.delete", "partition.delete_all", "format":
		beforeTree = systeminfo.Collect(s.nodeID).BlockTree
	}
	if input.Operation == "format" {
		beforePurposes, _ = s.state.ListStoragePurposes(r.Context())
	}

	switch input.Operation {
	case "unmount", "format", "partition.delete", "partition.delete_all":
		node, ok := blockNodeByPath(beforeTree, input.Device)
		if ok {
			pools, listErr := s.state.ListNASPools(r.Context())
			if listErr != nil {
				writeAPIError(w, r, http.StatusInternalServerError, "storage_usage_unavailable", "storage usage is unavailable", nil)
				return
			}
			usage := storageUsageForNode(node, pools)
			if len(usage) > 0 {
				writeAPIError(
					w,
					r,
					http.StatusConflict,
					"storage_in_use",
					"storage is used by a Home-AI file pool and cannot be unmounted, formatted or deleted",
					map[string]any{"used_by": usage},
				)
				return
			}
		}
	}

	message, err := storage.Execute(r.Context(), storage.Request{
		Operation:  input.Operation,
		Device:     input.Device,
		Mountpoint: input.Mountpoint,
		Filesystem: input.Filesystem,
		Label:      input.Label,
		Confirm:    input.Confirm,
		SizeMiB:    input.SizeMiB,
	})
	if err != nil {
		s.logger.Error("storage operation failed", "operation", input.Operation, "device", input.Device, "error", err)
		writeAPIError(w, r, http.StatusBadGateway, "storage_operation_failed", err.Error(), nil)
		return
	}

	response := map[string]any{"message": message}
	if input.Operation == "mount" {
		mountpoint := strings.TrimSpace(input.Mountpoint)
		if mountpoint == "" {
			mountpoint = filepath.Join("/mnt/home-ai-core", filepath.Base(input.Device))
		}
		response["mountpoint"] = filepath.Clean(mountpoint)
	}
	var purposeWarning string

	switch input.Operation {
	case "partition.create":
		if input.Purpose != "" && input.Purpose != "none" {
			afterTree := systeminfo.Collect(s.nodeID).BlockTree
			before := partitionPathsForDisk(beforeTree, input.Device)
			after := partitionPathsForDisk(afterTree, input.Device)
			var created []systeminfo.BlockNode
			for path, node := range after {
				if _, existed := before[path]; !existed {
					created = append(created, node)
				}
			}
			if len(created) == 1 {
				record, assignErr := s.state.SetStoragePurpose(
					r.Context(),
					created[0].Path,
					created[0].UUID,
					input.Purpose,
					time.Now().UTC(),
				)
				if assignErr != nil {
					purposeWarning = fmt.Sprintf("partition was created but purpose could not be saved: %v", assignErr)
				} else {
					response["device"] = record.DevicePath
					response["purpose"] = record.Purpose
					response["purpose_assigned"] = true
					s.realtime.Publish("storage.purpose.changed", map[string]any{
						"device":  record.DevicePath,
						"purpose": record.Purpose,
					}, requestIDFromContext(r.Context()))
				}
			} else {
				purposeWarning = "partition was created but the new partition could not be identified for purpose assignment"
			}
		}

	case "format":
		beforeNode, ok := blockNodeByPath(beforeTree, input.Device)
		if ok {
			if record, found := storagePurposeRecordForNode(beforePurposes, beforeNode); found {
				afterTree := systeminfo.Collect(s.nodeID).BlockTree
				if afterNode, present := blockNodeByPath(afterTree, input.Device); present {
					if _, rebindErr := s.state.SetStoragePurpose(
						r.Context(),
						afterNode.Path,
						afterNode.UUID,
						record.Purpose,
						time.Now().UTC(),
					); rebindErr != nil {
						s.logger.Warn("failed to rebind storage purpose after format", "device", input.Device, "error", rebindErr)
					}
				}
			}
		}

	case "partition.delete":
		if node, ok := blockNodeByPath(beforeTree, input.Device); ok {
			if clearErr := s.state.ClearStoragePurpose(r.Context(), node.Path, node.UUID); clearErr != nil {
				s.logger.Warn("failed to clear storage purpose after partition delete", "device", input.Device, "error", clearErr)
			}
		}

	case "partition.delete_all":
		for _, node := range partitionPathsForDisk(beforeTree, input.Device) {
			if clearErr := s.state.ClearStoragePurpose(r.Context(), node.Path, node.UUID); clearErr != nil {
				s.logger.Warn("failed to clear storage purpose after disk partition cleanup", "device", node.Path, "error", clearErr)
			}
		}
	}

	if purposeWarning != "" {
		response["warning"] = purposeWarning
	}

	s.security.RecordAudit(
		r.Context(),
		s.securityRequestContext(r),
		actor,
		"storage."+input.Operation,
		"block_device",
		input.Device,
		"success",
		map[string]any{
			"mountpoint": input.Mountpoint,
			"filesystem": input.Filesystem,
			"label":      input.Label,
			"size_mib":   input.SizeMiB,
			"purpose":    input.Purpose,
		},
	)
	writeJSON(w, http.StatusOK, response)
}

func storagePurposeRecordForNode(records []state.StoragePurposeRecord, node systeminfo.BlockNode) (state.StoragePurposeRecord, bool) {
	for _, record := range records {
		if node.UUID != "" && record.FilesystemUUID == node.UUID {
			return record, true
		}
		if record.DevicePath == node.Path {
			return record, true
		}
	}
	return state.StoragePurposeRecord{}, false
}
