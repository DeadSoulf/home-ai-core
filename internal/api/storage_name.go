package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/systeminfo"
)

func (s *server) storageName(
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
		Device string `json:"device"`
		Name   string `json:"name"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_storage_name", "invalid disk name request", nil)
		return
	}

	input.Device = strings.TrimSpace(input.Device)
	input.Name = strings.TrimSpace(input.Name)
	if input.Device == "" {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_storage_name", "disk device is required", nil)
		return
	}
	if len([]rune(input.Name)) > 64 {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_storage_name", "disk name must be 64 characters or fewer", nil)
		return
	}

	info := systeminfo.Collect(s.nodeID)
	var stableID string
	for _, disk := range info.BlockTree {
		if disk.Type == "disk" && disk.Path == input.Device {
			stableID = systeminfo.DiskStableID(disk.Path, disk.Serial)
			break
		}
	}
	if stableID == "" {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_storage_name", "device is not a physical disk", nil)
		return
	}

	if err := s.state.SetDiskName(r.Context(), stableID, input.Name); err != nil {
		s.logger.Error("save disk name", "device", input.Device, "error", err)
		writeAPIError(w, r, http.StatusInternalServerError, "storage_name_failed", "failed to save disk name", nil)
		return
	}

	s.security.RecordAudit(
		r.Context(),
		s.securityRequestContext(r),
		actor,
		"storage.disk.rename",
		"block_device",
		input.Device,
		"success",
		map[string]any{"display_name": input.Name},
	)
	writeJSON(w, http.StatusOK, map[string]any{"name": input.Name})
}
