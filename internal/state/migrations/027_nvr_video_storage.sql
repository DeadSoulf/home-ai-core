-- Extend existing storage-purpose assignments without losing UUID bindings.
CREATE TABLE storage_purposes_new (
    device_path TEXT PRIMARY KEY,
    filesystem_uuid TEXT NOT NULL DEFAULT '',
    purpose TEXT NOT NULL CHECK (purpose IN ('files','nvr')),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
INSERT INTO storage_purposes_new(device_path, filesystem_uuid, purpose, created_at, updated_at)
SELECT device_path, filesystem_uuid, purpose, created_at, updated_at
FROM storage_purposes;
DROP TABLE storage_purposes;
ALTER TABLE storage_purposes_new RENAME TO storage_purposes;
CREATE UNIQUE INDEX idx_storage_purposes_filesystem_uuid
ON storage_purposes(filesystem_uuid)
WHERE filesystem_uuid <> '';
CREATE INDEX idx_storage_purposes_purpose ON storage_purposes(purpose);

-- Activation is separate from declaring a disk's intended use. It must be
-- recorded only after the helper has safely migrated the recordings and
-- started the new, verified NVR container with the dedicated disk mounted.
CREATE TABLE nvr_video_storage (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    device_path TEXT NOT NULL,
    filesystem_uuid TEXT NOT NULL,
    mountpoint TEXT NOT NULL,
    active_at TEXT NOT NULL
);
