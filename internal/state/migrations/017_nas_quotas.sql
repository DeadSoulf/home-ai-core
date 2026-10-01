ALTER TABLE nas_folders ADD COLUMN quota_bytes INTEGER NOT NULL DEFAULT 0 CHECK (quota_bytes >= 0);
ALTER TABLE nas_folders ADD COLUMN hard_quota_bytes INTEGER NOT NULL DEFAULT 0 CHECK (hard_quota_bytes >= 0);

CREATE TABLE nas_user_quotas (
    user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    quota_bytes INTEGER NOT NULL DEFAULT 0 CHECK (quota_bytes >= 0),
    updated_at TEXT NOT NULL
);

-- These IDs are never reused: stale filesystem project accounting cannot be
-- attached to a newly created folder after metadata cleanup.
CREATE TABLE nas_quota_projects (
    project_id INTEGER PRIMARY KEY AUTOINCREMENT,
    folder_id TEXT NOT NULL UNIQUE
);
INSERT INTO nas_quota_projects(folder_id) SELECT id FROM nas_folders ORDER BY id;

CREATE TABLE nas_settings (name TEXT PRIMARY KEY, value TEXT NOT NULL);

