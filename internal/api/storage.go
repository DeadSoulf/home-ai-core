package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/storage"
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
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_storage_request", "invalid storage request", nil)
		return
	}

	input.Operation = strings.TrimSpace(input.Operation)
	input.Device = strings.TrimSpace(input.Device)
	if input.Operation == "" || input.Device == "" {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_storage_request", "storage operation and device are required", nil)
		return
	}

	message, err := storage.Execute(r.Context(), storage.Request{
		Operation:  input.Operation,
		Device:     input.Device,
		Mountpoint: input.Mountpoint,
		Filesystem: input.Filesystem,
		Label:      input.Label,
		Confirm:    input.Confirm,
	})
	if err != nil {
		s.logger.Error("storage operation failed", "operation", input.Operation, "device", input.Device, "error", err)
		writeAPIError(w, r, http.StatusBadGateway, "storage_operation_failed", err.Error(), nil)
		return
	}

	s.security.RecordAudit(
		r.Context(),
		requestSecurityContext(r),
		actor,
		"storage."+input.Operation,
		"block_device",
		input.Device,
		"success",
		map[string]any{"mountpoint": input.Mountpoint, "filesystem": input.Filesystem},
	)
	writeJSON(w, http.StatusOK, map[string]any{"message": message})
}
