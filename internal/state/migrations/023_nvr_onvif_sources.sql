-- Persist ONVIF management metadata separately from the RTSP media address used by runtime workers.

CREATE TABLE nvr_onvif_sources (
    camera_id TEXT PRIMARY KEY REFERENCES nvr_cameras(id) ON DELETE CASCADE,
    device_endpoint TEXT NOT NULL,
    main_profile_token TEXT NOT NULL,
    sub_profile_token TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX idx_nvr_onvif_sources_endpoint
ON nvr_onvif_sources(device_endpoint);
