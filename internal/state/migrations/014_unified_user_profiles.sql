CREATE TABLE user_permission_overrides (
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    permission_name TEXT NOT NULL REFERENCES permissions(name) ON DELETE CASCADE,
    allowed INTEGER NOT NULL CHECK (allowed IN (0, 1)),
    PRIMARY KEY (user_id, permission_name)
);

CREATE INDEX idx_user_permission_overrides_permission
ON user_permission_overrides(permission_name, allowed);

INSERT INTO roles(id, name, description, created_at) VALUES
    ('role_administrator', 'administrator', 'Home-AI administrator', CURRENT_TIMESTAMP),
    ('role_parent', 'parent', 'Home-AI parent profile', CURRENT_TIMESTAMP),
    ('role_child', 'child', 'Home-AI child profile', CURRENT_TIMESTAMP),
    ('role_friend', 'friend', 'Home-AI friend profile', CURRENT_TIMESTAMP),
    ('role_guest', 'guest', 'Home-AI guest profile', CURRENT_TIMESTAMP);

-- Administrators are also treated as full-access dynamically by the security
-- service so permissions introduced by future modules are included
-- automatically. These rows keep the database catalog readable.
INSERT INTO role_permissions(role_id, permission_name)
SELECT 'role_administrator', name FROM permissions;

INSERT INTO role_permissions(role_id, permission_name)
SELECT 'role_parent', name
FROM permissions
WHERE name IN (
    'system.read',
    'events.read',
    'security.self.read',
    'security.sessions.manage',
    'security.users.read',
    'jobs.read',
    'modules.read',
    'updates.read',
    'network.read'
);

INSERT INTO role_permissions(role_id, permission_name)
SELECT 'role_child', name
FROM permissions
WHERE name IN (
    'system.read',
    'events.read',
    'security.self.read',
    'security.sessions.manage'
);

INSERT INTO role_permissions(role_id, permission_name)
SELECT r.id, p.name
FROM roles r
CROSS JOIN permissions p
WHERE r.id IN ('role_friend', 'role_guest')
  AND p.name IN ('security.self.read', 'security.sessions.manage');

-- Existing shared-folder grants were role-wide for legacy members. Convert
-- them to per-user grants before removing the role-wide rows so existing
-- users keep exactly the access they had.
INSERT OR IGNORE INTO user_resource_permissions(
    user_id,
    permission_name,
    resource_type,
    resource_id
)
SELECT
    ur.user_id,
    rrp.permission_name,
    rrp.resource_type,
    rrp.resource_id
FROM role_resource_permissions rrp
JOIN user_roles ur ON ur.role_id = rrp.role_id
WHERE rrp.role_id = 'role_member';

DELETE FROM role_resource_permissions
WHERE role_id = 'role_member';
