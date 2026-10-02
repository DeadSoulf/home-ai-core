-- NVR control-plane foundation. Media payloads remain outside SQLite.

INSERT INTO permissions(name, description) VALUES
    ('camera.list', 'List cameras available through the Home-AI NVR'),
    ('camera.live', 'View live video for an allowed camera'),
    ('camera.archive', 'Read archive and timeline for an allowed camera'),
    ('camera.export', 'Create snapshots and export clips for an allowed camera'),
    ('camera.ptz', 'Control PTZ for an allowed camera'),
    ('camera.manage', 'Manage an allowed camera configuration'),
    ('nvr.storage.manage', 'Manage NVR video archive storage and retention'),
    ('nvr.settings.manage', 'Manage global NVR settings');

INSERT INTO role_permissions(role_id, permission_name)
SELECT 'role_owner', name
FROM permissions
WHERE name IN (
    'camera.list',
    'camera.live',
    'camera.archive',
    'camera.export',
    'camera.ptz',
    'camera.manage',
    'nvr.storage.manage',
    'nvr.settings.manage'
);

CREATE TABLE nvr_cameras (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE COLLATE NOCASE,
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    source_type TEXT NOT NULL CHECK (source_type IN ('rtsp', 'onvif')),
    address TEXT NOT NULL,
    credential_ref TEXT CHECK (
        credential_ref IS NULL OR
        (length(credential_ref) BETWEEN 5 AND 128 AND substr(credential_ref, 1, 4) = 'sec_')
    ),
    transport TEXT NOT NULL DEFAULT 'tcp' CHECK (transport IN ('tcp', 'udp')),
    recording_mode TEXT NOT NULL DEFAULT 'off' CHECK (recording_mode IN ('off', 'continuous', 'motion')),
    audio_enabled INTEGER NOT NULL DEFAULT 0 CHECK (audio_enabled IN (0, 1)),
    created_by TEXT REFERENCES users(id) ON DELETE SET NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX idx_nvr_cameras_enabled_name
ON nvr_cameras(enabled, name COLLATE NOCASE);

CREATE TABLE nvr_stream_profiles (
    id TEXT PRIMARY KEY,
    camera_id TEXT NOT NULL REFERENCES nvr_cameras(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('main', 'sub')),
    source_uri TEXT NOT NULL,
    codec TEXT NOT NULL DEFAULT '',
    width INTEGER NOT NULL DEFAULT 0 CHECK (width >= 0),
    height INTEGER NOT NULL DEFAULT 0 CHECK (height >= 0),
    fps REAL NOT NULL DEFAULT 0 CHECK (fps >= 0),
    bitrate_bps INTEGER NOT NULL DEFAULT 0 CHECK (bitrate_bps >= 0),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(camera_id, role)
);

CREATE INDEX idx_nvr_stream_profiles_camera
ON nvr_stream_profiles(camera_id, role);

CREATE TABLE nvr_storage_targets (
    id TEXT PRIMARY KEY,
    device_path TEXT NOT NULL,
    filesystem_uuid TEXT NOT NULL DEFAULT '',
    mountpoint TEXT NOT NULL,
    reserve_percent INTEGER NOT NULL DEFAULT 5 CHECK (reserve_percent BETWEEN 1 AND 50),
    active INTEGER NOT NULL DEFAULT 0 CHECK (active IN (0, 1)),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_nvr_storage_target_active
ON nvr_storage_targets(active)
WHERE active = 1;

CREATE INDEX idx_nvr_storage_target_identity
ON nvr_storage_targets(filesystem_uuid, device_path);

CREATE TABLE nvr_recording_segments (
    id TEXT PRIMARY KEY,
    camera_id TEXT NOT NULL REFERENCES nvr_cameras(id) ON DELETE CASCADE,
    storage_target_id TEXT NOT NULL REFERENCES nvr_storage_targets(id) ON DELETE RESTRICT,
    start_at TEXT NOT NULL,
    end_at TEXT NOT NULL,
    relative_path TEXT NOT NULL,
    codec TEXT NOT NULL DEFAULT '',
    width INTEGER NOT NULL DEFAULT 0 CHECK (width >= 0),
    height INTEGER NOT NULL DEFAULT 0 CHECK (height >= 0),
    size_bytes INTEGER NOT NULL DEFAULT 0 CHECK (size_bytes >= 0),
    protected INTEGER NOT NULL DEFAULT 0 CHECK (protected IN (0, 1)),
    status TEXT NOT NULL DEFAULT 'complete'
        CHECK (status IN ('writing', 'complete', 'recovering', 'corrupt', 'missing')),
    created_at TEXT NOT NULL,
    UNIQUE(storage_target_id, relative_path)
);

CREATE INDEX idx_nvr_segments_camera_time
ON nvr_recording_segments(camera_id, start_at, end_at);

CREATE INDEX idx_nvr_segments_retention
ON nvr_recording_segments(protected, start_at)
WHERE status = 'complete';

CREATE TABLE nvr_review_events (
    id TEXT PRIMARY KEY,
    camera_id TEXT NOT NULL REFERENCES nvr_cameras(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('motion', 'bookmark', 'system')),
    severity TEXT NOT NULL DEFAULT 'info' CHECK (severity IN ('info', 'notice', 'alert')),
    start_at TEXT NOT NULL,
    end_at TEXT NOT NULL,
    thumbnail_ref TEXT,
    acknowledged_at TEXT,
    created_at TEXT NOT NULL
);

CREATE INDEX idx_nvr_review_events_camera_time
ON nvr_review_events(camera_id, start_at DESC);

CREATE INDEX idx_nvr_review_events_unacknowledged
ON nvr_review_events(acknowledged_at, start_at DESC);
