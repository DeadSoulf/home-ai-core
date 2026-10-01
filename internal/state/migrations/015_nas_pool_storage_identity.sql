ALTER TABLE nas_pools ADD COLUMN storage_device_path TEXT NOT NULL DEFAULT '';
ALTER TABLE nas_pools ADD COLUMN storage_filesystem_uuid TEXT NOT NULL DEFAULT '';

CREATE INDEX idx_nas_pools_storage_device_path
ON nas_pools(storage_device_path)
WHERE storage_device_path <> '';

CREATE INDEX idx_nas_pools_storage_filesystem_uuid
ON nas_pools(storage_filesystem_uuid)
WHERE storage_filesystem_uuid <> '';
