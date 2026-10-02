package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

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
			MediaRuntimeReady: s.nvr != nil && s.nvr.MediaProbeReady(),
			SecretStoreReady:  s.nvr != nil && s.nvr.SecretStoreReady(),
			FoundationStage:   nvrFoundationStage(s.nvr),
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


func nvrFoundationStage(service *nvrpkg.Service) string {
	if service == nil {
		return "nvr-0"
	}
	return "nvr-1-onboarding"
}

func (s *server) nvrEnabled(w http.ResponseWriter, r *http.Request) bool {
	item, err := s.modules.Get(r.Context(), nvrpkg.ModuleID)
	if err != nil {
		writeAPIError(w, r, http.StatusServiceUnavailable, "nvr_unavailable", "NVR module is unavailable", nil)
		return false
	}
	if item.Status != "enabled" {
		writeAPIError(w, r, http.StatusConflict, "nvr_disabled", "NVR module is disabled", nil)
		return false
	}
	if s.nvr == nil {
		writeAPIError(w, r, http.StatusServiceUnavailable, "nvr_runtime_unavailable", "NVR onboarding runtime is unavailable", nil)
		return false
	}
	return true
}

func (s *server) nvrCameraCreate(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	source authSource,
) {
	if !s.nvrEnabled(w, r) {
		return
	}
	if !actor.Has(nvrpkg.PermissionCameraManage) {
		writeAPIError(w, r, http.StatusForbidden, "permission_denied", "camera management permission required", nil)
		return
	}
	if !validMutationCSRF(actor, source, r) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}
	var input nvrpkg.CameraInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	camera, probe, err := s.nvr.CreateCamera(r.Context(), actor.ID, input)
	if err != nil {
		s.writeNVRError(w, r, err)
		return
	}
	s.security.RecordAudit(
		context.WithoutCancel(r.Context()),
		s.securityRequestContext(r),
		actor,
		"nvr.camera.create",
		"camera",
		camera.ID,
		"success",
		map[string]any{
			"name":           camera.Name,
			"transport":      camera.Transport,
			"recording_mode": camera.RecordingMode,
			"codec":          probe.Codec,
			"width":          probe.Width,
			"height":         probe.Height,
		},
	)
	writeJSON(w, http.StatusCreated, map[string]any{"camera": camera, "probe": probe})
}

func (s *server) nvrCameraTest(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	source authSource,
) {
	if !s.nvrEnabled(w, r) {
		return
	}
	if !actor.Has(nvrpkg.PermissionCameraManage) {
		writeAPIError(w, r, http.StatusForbidden, "permission_denied", "camera management permission required", nil)
		return
	}
	if !validMutationCSRF(actor, source, r) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}
	var input nvrpkg.CameraInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	probe, err := s.nvr.TestCamera(r.Context(), input)
	if err != nil {
		s.writeNVRError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "probe": probe})
}

func (s *server) nvrCameraResource(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	source authSource,
) {
	if !s.nvrEnabled(w, r) {
		return
	}
	cameraID := strings.TrimSpace(r.PathValue("cameraID"))
	if cameraID == "" {
		s.notFound(w, r)
		return
	}
	if !actor.Allows(nvrpkg.PermissionCameraManage, "camera", cameraID) {
		writeAPIError(w, r, http.StatusForbidden, "permission_denied", "camera management permission required", nil)
		return
	}

	switch r.Method {
	case http.MethodGet:
		camera, err := s.nvr.CameraConfig(r.Context(), cameraID)
		if err != nil {
			s.writeNVRError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"camera": camera})
	case http.MethodPut:
		if !validMutationCSRF(actor, source, r) {
			writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
			return
		}
		var input nvrpkg.CameraInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeAPIError(w, r, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		camera, probe, err := s.nvr.UpdateCamera(r.Context(), cameraID, input)
		if err != nil {
			s.writeNVRError(w, r, err)
			return
		}
		s.security.RecordAudit(
			context.WithoutCancel(r.Context()),
			s.securityRequestContext(r),
			actor,
			"nvr.camera.update",
			"camera",
			cameraID,
			"success",
			map[string]any{
				"name":           camera.Name,
				"enabled":        camera.Enabled,
				"transport":      camera.Transport,
				"recording_mode": camera.RecordingMode,
				"codec":          probe.Codec,
				"width":          probe.Width,
				"height":         probe.Height,
			},
		)
		writeJSON(w, http.StatusOK, map[string]any{"camera": camera, "probe": probe})
	case http.MethodDelete:
		if !validMutationCSRF(actor, source, r) {
			writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
			return
		}
		if err := s.nvr.DeleteCamera(r.Context(), cameraID); err != nil {
			s.writeNVRError(w, r, err)
			return
		}
		s.security.RecordAudit(
			context.WithoutCancel(r.Context()),
			s.securityRequestContext(r),
			actor,
			"nvr.camera.delete",
			"camera",
			cameraID,
			"success",
			nil,
		)
		writeJSON(w, http.StatusOK, map[string]any{"deleted": cameraID})
	default:
		methodNotAllowed(w, r, http.MethodGet, http.MethodPut, http.MethodDelete)
	}
}

func (s *server) nvrCameraExistingTest(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	source authSource,
) {
	if !s.nvrEnabled(w, r) {
		return
	}
	cameraID := strings.TrimSpace(r.PathValue("cameraID"))
	if !actor.Allows(nvrpkg.PermissionCameraManage, "camera", cameraID) {
		writeAPIError(w, r, http.StatusForbidden, "permission_denied", "camera management permission required", nil)
		return
	}
	if !validMutationCSRF(actor, source, r) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}
	probe, err := s.nvr.TestExistingCamera(r.Context(), cameraID)
	if err != nil {
		s.writeNVRError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "probe": probe})
}

func (s *server) writeNVRError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, state.ErrNVRCameraNotFound):
		writeAPIError(w, r, http.StatusNotFound, "camera_not_found", "camera not found", nil)
	case errors.Is(err, nvrpkg.ErrMediaRuntimeUnavailable):
		writeAPIError(w, r, http.StatusServiceUnavailable, "nvr_media_runtime_unavailable", "ffprobe is not available on this Home-AI node", nil)
	case errors.Is(err, nvrpkg.ErrSecretStoreUnavailable):
		writeAPIError(w, r, http.StatusServiceUnavailable, "nvr_secret_store_unavailable", "camera credential store is unavailable", nil)
	case errors.Is(err, nvrpkg.ErrRTSPAuthentication):
		writeAPIError(w, r, http.StatusBadGateway, "camera_authentication_failed", "camera authentication failed", nil)
	case errors.Is(err, nvrpkg.ErrRTSPConnection):
		writeAPIError(w, r, http.StatusBadGateway, "camera_connection_failed", "camera RTSP connection failed", nil)
	case errors.Is(err, nvrpkg.ErrRTSPNoVideo):
		writeAPIError(w, r, http.StatusUnprocessableEntity, "camera_no_video", "camera source has no video stream", nil)
	case errors.Is(err, context.DeadlineExceeded):
		writeAPIError(w, r, http.StatusGatewayTimeout, "camera_probe_timeout", "camera connection test timed out", nil)
	default:
		message := strings.TrimSpace(err.Error())
		if message == "" || strings.Contains(strings.ToLower(message), "password") {
			message = "camera request failed"
		}
		writeAPIError(w, r, http.StatusBadRequest, "camera_invalid", message, nil)
	}
}
