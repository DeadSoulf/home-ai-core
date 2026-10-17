package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/DeadSoulf/home-ai-core/internal/modules"
	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/state"
)

type storagePurposeChoice struct {
	ID string `json:"id"`
	Name string `json:"name"`
	ModuleID string `json:"module_id,omitempty"`
}

// Storage purposes are owned by Files or a currently registered module. This
// catalog is read-only: selecting a purpose never formats or mounts the disk.
func (s *server) storagePurposeOptions(w http.ResponseWriter, r *http.Request, _ security.Actor, _ authSource) {
	options := []storagePurposeChoice{{ID: state.StoragePurposeFiles, Name: "Файлы"}}
	if s.modules != nil {
		installed, err := s.modules.List(r.Context())
		if err != nil {
			writeAPIError(w, r, http.StatusServiceUnavailable, "module_registry_unavailable", "Module registry is unavailable", nil)
			return
		}
		for _, item := range installed {
			if item.Status != "enabled" && item.Status != "disabled" {
				continue
			}
			options = append(options, storagePurposeChoice{
				ID: "module:" + item.Manifest.ID,
				Name: item.Manifest.Name,
				ModuleID: item.Manifest.ID,
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"purposes": options})
}

func (s *server) validateAssignedModulePurpose(ctx context.Context, raw string) (string, error) {
	purpose, err := state.NormalizeStoragePurpose(raw)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(purpose, "module:") {
		return purpose, nil
	}
	if s.modules == nil {
		return "", errors.New("module registry is unavailable")
	}
	id := strings.TrimPrefix(purpose, "module:")
	item, err := s.modules.Get(ctx, id)
	if err != nil || (item.Status != "enabled" && item.Status != "disabled") {
		return "", errors.New("storage purpose requires an installed module")
	}
	return purpose, nil
}

func assignedStoragePurposeForNode(nodePath, nodeUUID string, records []state.StoragePurposeRecord) string {
	for _, record := range records {
		if nodeUUID != "" && record.FilesystemUUID != "" && record.FilesystemUUID == nodeUUID {
			return record.Purpose
		}
		if record.DevicePath == nodePath {
			return record.Purpose
		}
	}
	return ""
}

func (s *server) modulePurposeInUse(ctx context.Context, purpose string) bool {
	if !strings.HasPrefix(purpose, "module:") || s.modules == nil {
		return false
	}
	id := strings.TrimPrefix(purpose, "module:")
	item, err := s.modules.Get(ctx, id)
	return err == nil && (item.Status == "enabled" || item.Status == "disabled")
}

// Strictly forbid modifying disks assigned to installed modules until a
// controlled detach/migration workflow can prove that no Docker bind mount
// or video recorder still depends on the device.
func (s *server) ensureStorageDeviceNotUsedByModule(ctx context.Context, device, uuid string, records []state.StoragePurposeRecord) error {
	purpose := assignedStoragePurposeForNode(device, uuid, records)
	if s.modulePurposeInUse(ctx, purpose) {
		return errors.New("partition is reserved by an installed module; detach its data safely before changing or removing the device")
	}
	return nil
}

var _ = modules.ErrModuleNotFound
