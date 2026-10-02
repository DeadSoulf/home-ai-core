INSERT OR IGNORE INTO permissions(name, description)
VALUES ('modules.manage', 'Enable, disable and restart controllable modules');

INSERT OR IGNORE INTO role_permissions(role_id, permission_name)
SELECT 'role_owner', 'modules.manage';
