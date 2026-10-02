package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/DeadSoulf/home-ai-core/internal/modules"
	nvrpkg "github.com/DeadSoulf/home-ai-core/internal/nvr"
	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/state"
)

type nvrCameraState interface {
	ListNVRCameras(context.Context) ([]state.NVRCameraRecord, error)
}

func (s *server) nvrStatus(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	_ authSource,
) {
	item, err := s.modules.Get(r.Context(), nvrpkg.ModuleID)
	if errors.Is(err, modules.ErrModuleNotFound) {
		writeAPIError(w, r, http.StatusServiceUnavailable, "nvr_unavailable", "NVR module is unavailable", nil)
		return
	}
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "modules_unavailable", "module registry is unavailable", nil)
		return
	}

	cameras, err := s.visibleNVRCameras(r.Context(), actor)
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "nvr_state_unavailable", "NVR state is unavailable", nil)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"nvr": nvrpkg.Status{
			ModuleID:          nvrpkg.ModuleID,
			State:             item.Status,
			Version:           item.Manifest.Version,
			CameraCount:       len(cameras),
			MediaRuntimeReady: false,
			SecretStoreReady:  false,
			FoundationStage:   "nvr-0",
		},
	})
}

func (s *server) nvrCameras(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	_ authSource,
) {
	item, err := s.modules.Get(r.Context(), nvrpkg.ModuleID)
	if err != nil {
		writeAPIError(w, r, http.StatusServiceUnavailable, "nvr_unavailable", "NVR module is unavailable", nil)
		return
	}
	if item.Status != "enabled" {
		writeAPIError(w, r, http.StatusConflict, "nvr_disabled", "NVR module is disabled", nil)
		return
	}

	cameras, err := s.visibleNVRCameras(r.Context(), actor)
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "nvr_state_unavailable", "NVR state is unavailable", nil)
		return
	}
	out := make([]nvrpkg.CameraSummary, 0, len(cameras))
	for _, camera := range cameras {
		out = append(out, nvrpkg.CameraSummary{
			ID:             camera.ID,
			Name:           camera.Name,
			Enabled:        camera.Enabled,
			SourceType:     camera.SourceType,
			Transport:      camera.Transport,
			RecordingMode:  camera.RecordingMode,
			AudioEnabled:   camera.AudioEnabled,
			HasCredentials: camera.CredentialRef != "",
			CreatedAt:      camera.CreatedAt,
			UpdatedAt:      camera.UpdatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"cameras": out})
}

func (s *server) visibleNVRCameras(ctx context.Context, actor security.Actor) ([]state.NVRCameraRecord, error) {
	store, ok := s.state.(nvrCameraState)
	if !ok {
		return nil, errors.New("NVR camera state is not available")
	}
	cameras, err := store.ListNVRCameras(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]state.NVRCameraRecord, 0, len(cameras))
	for _, camera := range cameras {
		if actor.Has(nvrpkg.PermissionCameraList) ||
			actor.Allows(nvrpkg.PermissionCameraLive, "camera", camera.ID) ||
			actor.Allows(nvrpkg.PermissionCameraArchive, "camera", camera.ID) ||
			actor.Allows(nvrpkg.PermissionCameraExport, "camera", camera.ID) ||
			actor.Allows(nvrpkg.PermissionCameraPTZ, "camera", camera.ID) ||
			actor.Allows(nvrpkg.PermissionCameraManage, "camera", camera.ID) {
			out = append(out, camera)
		}
	}
	return out, nil
}
