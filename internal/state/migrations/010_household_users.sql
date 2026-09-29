INSERT INTO roles(id, name, description, created_at)
VALUES ('role_member', 'member', 'Home-AI household member', CURRENT_TIMESTAMP);

INSERT INTO permissions(name, description)
VALUES ('security.users.read', 'Read platform user accounts');

INSERT INTO role_permissions(role_id, permission_name)
VALUES ('role_owner', 'security.users.read');

INSERT INTO role_permissions(role_id, permission_name)
SELECT 'role_member', name
FROM permissions
WHERE name IN ('security.self.read', 'security.sessions.manage');
