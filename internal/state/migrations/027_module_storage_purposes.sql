-- Add per-module disk assignments without changing existing Files data.
-- The API verifies the module identity and protects in-use devices.
CREATE TABLE storage_purposes_v2 (
    device_path TEXT PRIMARY KEY,
    filesystem_uuid TEXT NOT NULL DEFAULT '',
    purpose TEXT NOT NULL CHECK (purpose = 'files' OR purpose GLOB 'module:*'),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
INSERT INTO storage_purposes_v2 (device_path,filesystem_uuid,purpose,created_at,updated_at)
SELECT device_path,filesystem_uuid,purpose,created_at,updated_at FROM storage_purposes;
DROP TABLE storage_purposes;
ALTER TABLE storage_purposes_v2 RENAME TO storage_purposes;
CREATE UNIQUE INDEX idx_storage_purposes_filesystem_uuid
ON storage_purposes(filesystem_uuid) WHERE filesystem_uuid <> '';
CREATE INDEX idx_storage_purposes_purpose ON storage_purposes(purpose);
