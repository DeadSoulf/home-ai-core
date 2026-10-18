-- Preserve all existing Files assignments while allowing a separate,
-- exclusive physical storage purpose for the NVR recording archive.
CREATE TABLE storage_purposes_next (
    device_path TEXT PRIMARY KEY,
    filesystem_uuid TEXT NOT NULL DEFAULT '',
    purpose TEXT NOT NULL CHECK (purpose IN ('files', 'nvr')),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
INSERT INTO storage_purposes_next (device_path, filesystem_uuid, purpose, created_at, updated_at)
SELECT device_path, filesystem_uuid, purpose, created_at, updated_at
FROM storage_purposes;
DROP TABLE storage_purposes;
ALTER TABLE storage_purposes_next RENAME TO storage_purposes;
CREATE UNIQUE INDEX idx_storage_purposes_filesystem_uuid
ON storage_purposes(filesystem_uuid)
WHERE filesystem_uuid <> '';
CREATE INDEX idx_storage_purposes_purpose
ON storage_purposes(purpose);
