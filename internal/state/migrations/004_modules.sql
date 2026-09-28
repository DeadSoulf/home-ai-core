CREATE TABLE modules (
    id TEXT PRIMARY KEY,
    version TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('registered', 'enabled', 'disabled', 'error')),
    manifest_json TEXT NOT NULL,
    error_message TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX idx_modules_status ON modules(status);

INSERT INTO permissions(name, description) VALUES
    ('modules.read', 'Read module registry and manifest metadata'),
    ('modules.manage', 'Manage module lifecycle and configuration');

INSERT INTO role_permissions(role_id, permission_name)
SELECT 'role_owner', name
FROM permissions
WHERE name IN ('modules.read', 'modules.manage');
