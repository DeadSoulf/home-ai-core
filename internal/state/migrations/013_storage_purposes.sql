CREATE TABLE storage_purposes (
    device_path TEXT PRIMARY KEY,
    filesystem_uuid TEXT NOT NULL DEFAULT '',
    purpose TEXT NOT NULL CHECK (purpose = 'files'),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_storage_purposes_filesystem_uuid
ON storage_purposes(filesystem_uuid)
WHERE filesystem_uuid <> '';

CREATE INDEX idx_storage_purposes_purpose
ON storage_purposes(purpose);
