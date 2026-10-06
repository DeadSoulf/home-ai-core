-- Remove retired embedded product implementations from upgraded installations.
-- Product modules are external to Core; their state must not remain in Core SQLite.

DROP TABLE IF EXISTS nvr_onvif_sources;
DROP TABLE IF EXISTS nvr_review_events;
DROP TABLE IF EXISTS nvr_recording_segments;
DROP TABLE IF EXISTS nvr_stream_profiles;
DROP TABLE IF EXISTS nvr_storage_targets;
DROP TABLE IF EXISTS nvr_cameras;

DROP TABLE IF EXISTS ai_tool_actions;
DROP TABLE IF EXISTS ai_messages;
DROP TABLE IF EXISTS ai_conversations;

DELETE FROM modules
WHERE id IN ('ai.agent', 'ai.cloud', 'nvr');

DELETE FROM event_log
WHERE type LIKE 'ai.%'
   OR type LIKE 'nvr.%'
   OR type LIKE 'camera.%';

DELETE FROM jobs
WHERE type LIKE 'ai.%'
   OR type LIKE 'nvr.%'
   OR type LIKE 'camera.%';

DELETE FROM audit_events
WHERE action LIKE 'ai.%'
   OR action LIKE 'nvr.%'
   OR action LIKE 'camera.%'
   OR target_id IN ('ai.agent', 'ai.cloud', 'nvr');

DELETE FROM permissions
WHERE name LIKE 'camera.%'
   OR name LIKE 'nvr.%';

DELETE FROM storage_purposes
WHERE purpose <> 'files';

ALTER TABLE storage_purposes RENAME TO storage_purposes_before_module_cleanup;

CREATE TABLE storage_purposes (
    device_path TEXT PRIMARY KEY,
    filesystem_uuid TEXT NOT NULL DEFAULT '',
    purpose TEXT NOT NULL CHECK (purpose = 'files'),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

INSERT INTO storage_purposes(device_path, filesystem_uuid, purpose, created_at, updated_at)
SELECT device_path, filesystem_uuid, purpose, created_at, updated_at
FROM storage_purposes_before_module_cleanup
WHERE purpose = 'files';

DROP TABLE storage_purposes_before_module_cleanup;

CREATE UNIQUE INDEX idx_storage_purposes_filesystem_uuid
ON storage_purposes(filesystem_uuid)
WHERE filesystem_uuid <> '';

CREATE INDEX idx_storage_purposes_purpose
ON storage_purposes(purpose);
