CREATE TABLE nas_pools (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE COLLATE NOCASE,
    root_path TEXT NOT NULL UNIQUE,
    created_by TEXT REFERENCES users(id) ON DELETE SET NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE nas_folders (
    id TEXT PRIMARY KEY,
    pool_id TEXT NOT NULL REFERENCES nas_pools(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('private', 'shared')),
    owner_user_id TEXT REFERENCES users(id) ON DELETE CASCADE,
    relative_path TEXT NOT NULL,
    created_by TEXT REFERENCES users(id) ON DELETE SET NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(pool_id, relative_path),
    CHECK (
        (kind = 'private' AND owner_user_id IS NOT NULL) OR
        (kind = 'shared' AND owner_user_id IS NULL)
    )
);

CREATE INDEX idx_nas_folders_pool ON nas_folders(pool_id);
CREATE INDEX idx_nas_folders_owner ON nas_folders(owner_user_id);

INSERT INTO permissions(name, description) VALUES
    ('files.read', 'Read files and folder metadata within an allowed file resource'),
    ('files.write', 'Write files within an allowed file resource'),
    ('files.manage', 'Manage NAS pools, folders and file access');

INSERT INTO role_permissions(role_id, permission_name)
SELECT 'role_owner', name
FROM permissions
WHERE name IN ('files.read', 'files.write', 'files.manage');
