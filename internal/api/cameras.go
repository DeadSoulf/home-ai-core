package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/cameras"
	"github.com/DeadSoulf/home-ai-core/internal/security"
)

type CamerasService interface {
	Status() cameras.Status
	Refresh() cameras.Status
	TestLogin(context.Context, cameras.LoginRequest) (cameras.LoginResult, error)
	Discover(context.Context) ([]cameras.DiscoveredDevice, error)
}

func (s *server) camerasStatus(
	w http.ResponseWriter,
	r *http.Request,
	_ security.Actor,
	_ authSource,
) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, r, http.MethodGet)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cameras": s.cameras.Status()})
}

func (s *server) camerasTestLogin(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	source authSource,
) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, r, http.MethodPost)
		return
	}
	if !validMutationCSRF(actor, source, r) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}
	item, err := s.modules.Get(r.Context(), cameras.ModuleID)
	if err != nil || item.Status != "enabled" {
		writeAPIError(w, r, http.StatusConflict, "cameras_module_disabled", "Cameras module is disabled", nil)
		return
	}

	var request cameras.LoginRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	result, err := s.cameras.TestLogin(r.Context(), request)
	if err != nil {
		switch {
		case errors.Is(err, cameras.ErrInvalidTarget):
			writeAPIError(w, r, http.StatusBadRequest, "camera_target_invalid", err.Error(), nil)
		case errors.Is(err, cameras.ErrSDKUnsupported):
			writeAPIError(w, r, http.StatusServiceUnavailable, "camera_sdk_unsupported", err.Error(), nil)
		case errors.Is(err, cameras.ErrSDKUnavailable):
			writeAPIError(w, r, http.StatusServiceUnavailable, "camera_sdk_unavailable", err.Error(), nil)
		default:
			var sdkErr cameras.SDKError
			if errors.As(err, &sdkErr) {
				switch sdkErr.Code {
				case 1:
					writeAPIError(w, r, http.StatusBadGateway, "camera_authentication_failed", "HCNetSDK rejected the camera username or password", map[string]any{"sdk_error_code": sdkErr.Code})
				case 7:
					writeAPIError(w, r, http.StatusBadGateway, "camera_connection_failed", "HCNetSDK could not connect to the camera SDK service", map[string]any{"sdk_error_code": sdkErr.Code})
				default:
					writeAPIError(w, r, http.StatusBadGateway, "camera_sdk_login_failed", "HCNetSDK camera login failed", map[string]any{"sdk_error_code": sdkErr.Code})
				}
				return
			}
			writeAPIError(w, r, http.StatusBadGateway, "camera_sdk_login_failed", "HCNetSDK camera login failed", nil)
		}
		return
	}

	s.security.RecordAudit(
		context.WithoutCancel(r.Context()),
		s.securityRequestContext(r),
		actor,
		"camera.sdk.test",
		"camera",
		result.Address,
		"success",
		map[string]any{
			"backend": result.Backend,
			"port":    result.Port,
		},
	)
	writeJSON(w, http.StatusOK, map[string]any{"result": result})
}


func (s *server) camerasInstallRuntime(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	source authSource,
) {
	if !validMutationCSRF(actor, source, r) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}
	// SDK archives are large and may be uploaded over slow remote links. The
	// server-wide ReadTimeout protects ordinary API requests, but would abort
	// this bounded upload before the body is received.
	controller := http.NewResponseController(w)
	if err := controller.SetReadDeadline(time.Time{}); err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "camera_sdk_upload_deadline", "could not prepare long SDK upload", nil)
		return
	}

	if r.ContentLength <= 0 || r.ContentLength > 256<<20 {
		writeAPIError(w, r, http.StatusRequestEntityTooLarge, "camera_sdk_archive_invalid", "HCNetSDK ZIP must be smaller than 256 MiB", nil)
		return
	}
	installer, ok := s.cameras.(interface {
		InstallRuntime(io.Reader, int64) (cameras.InstallResult, error)
	})
	if !ok {
		writeAPIError(w, r, http.StatusServiceUnavailable, "camera_sdk_install_unavailable", "HCNetSDK installation is unavailable", nil)
		return
	}
	result, err := installer.InstallRuntime(io.LimitReader(r.Body, (256<<20)+1), r.ContentLength)
	if err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "camera_sdk_install_failed", err.Error(), nil)
		return
	}
	s.security.RecordAudit(
		context.WithoutCancel(r.Context()),
		s.securityRequestContext(r),
		actor,
		"camera.sdk.install",
		"camera",
		"hcnetsdk",
		"success",
		map[string]any{"path": result.Path, "initialized": result.Status.SDK.Initialized},
	)
	writeJSON(w, http.StatusOK, map[string]any{"result": result})
}


func (s *server) camerasDiscover(w http.ResponseWriter, r *http.Request, _ security.Actor, _ authSource) {
	items, err := s.cameras.Discover(r.Context())
	if err != nil {
		writeAPIError(w, r, http.StatusBadGateway, "camera_discovery_failed", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": items})
}
