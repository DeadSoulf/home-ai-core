CREATE TABLE user_permissions (
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    permission_name TEXT NOT NULL REFERENCES permissions(name) ON DELETE CASCADE,
    PRIMARY KEY (user_id, permission_name)
);

CREATE INDEX idx_user_permissions_permission
ON user_permissions(permission_name, user_id);

-- Keep stable role IDs for existing installations while presenting the
-- product-facing household profile names.
UPDATE roles
SET name = 'administrator',
    description = 'Home-AI administrator with full access'
WHERE id = 'role_owner';

UPDATE roles
SET name = 'friend',
    description = 'Home-AI friend profile'
WHERE id = 'role_member';

-- Existing shared-folder grants belonged to the old member role. Preserve
-- that effective access as direct grants before role_member becomes the
-- customizable Friend profile.
INSERT OR IGNORE INTO user_resource_permissions(user_id, permission_name, resource_type, resource_id)
SELECT ur.user_id, rrp.permission_name, rrp.resource_type, rrp.resource_id
FROM user_roles ur
JOIN role_resource_permissions rrp ON rrp.role_id = ur.role_id
WHERE ur.role_id = 'role_member';

DELETE FROM role_resource_permissions
WHERE role_id = 'role_member';

INSERT OR IGNORE INTO roles(id, name, description, created_at) VALUES
    ('role_parent', 'parent', 'Home-AI parent profile', CURRENT_TIMESTAMP),
    ('role_child', 'child', 'Home-AI child profile', CURRENT_TIMESTAMP),
    ('role_guest', 'guest', 'Home-AI guest profile', CURRENT_TIMESTAMP);

-- Every human account can inspect its own identity and manage its own session.
-- Additional access is assigned per user so role templates can be customized
-- without changing every user that shares the same household profile.
INSERT OR IGNORE INTO role_permissions(role_id, permission_name)
SELECT r.id, p.name
FROM roles r
JOIN permissions p ON p.name IN ('security.self.read', 'security.sessions.manage')
WHERE r.id IN ('role_parent', 'role_child', 'role_guest');

-- The administrator remains the full-access profile. Existing migrations
-- already granted every current permission to role_owner; this makes the
-- invariant explicit for databases upgraded from any earlier state.
INSERT OR IGNORE INTO role_permissions(role_id, permission_name)
SELECT 'role_owner', name FROM permissions;
