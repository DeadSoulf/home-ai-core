package nvr

import "time"

const (
	PermissionCameraList     = "camera.list"
	PermissionCameraLive     = "camera.live"
	PermissionCameraArchive  = "camera.archive"
	PermissionCameraExport   = "camera.export"
	PermissionCameraPTZ      = "camera.ptz"
	PermissionCameraManage   = "camera.manage"
	PermissionStorageManage  = "nvr.storage.manage"
	PermissionSettingsManage = "nvr.settings.manage"
)

var CameraScopedPermissions = []string{
	PermissionCameraLive,
	PermissionCameraArchive,
	PermissionCameraExport,
	PermissionCameraPTZ,
	PermissionCameraManage,
}

type CameraSummary struct {
	ID             string              `json:"id"`
	Name           string              `json:"name"`
	Enabled        bool                `json:"enabled"`
	SourceType     string              `json:"source_type"`
	Transport      string              `json:"transport"`
	RecordingMode  string              `json:"recording_mode"`
	AudioEnabled   bool                `json:"audio_enabled"`
	HasCredentials bool                `json:"has_credentials"`
	Runtime        CameraRuntimeStatus `json:"runtime"`
	CreatedAt      time.Time           `json:"created_at"`
	UpdatedAt      time.Time           `json:"updated_at"`
}

type StreamProfile struct {
	ID         string    `json:"id"`
	CameraID   string    `json:"camera_id"`
	Role       string    `json:"role"`
	Codec      string    `json:"codec,omitempty"`
	Width      int       `json:"width,omitempty"`
	Height     int       `json:"height,omitempty"`
	FPS        float64   `json:"fps,omitempty"`
	BitrateBPS int64     `json:"bitrate_bps,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Status struct {
	ModuleID          string `json:"module_id"`
	State             string `json:"state"`
	Version           string `json:"version"`
	CameraCount       int    `json:"camera_count"`
	OnlineCount       int    `json:"online_count"`
	OfflineCount      int    `json:"offline_count"`
	SupervisorRunning bool   `json:"supervisor_running"`
	MediaRuntimeReady bool   `json:"media_runtime_ready"`
	LiveRuntimeReady  bool   `json:"live_runtime_ready"`
	ActiveLiveStreams int    `json:"active_live_streams"`
	SecretStoreReady  bool   `json:"secret_store_ready"`
	FoundationStage   string `json:"foundation_stage"`
}
