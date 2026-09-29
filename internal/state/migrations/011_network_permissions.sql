INSERT INTO permissions(name, description)
VALUES
  ('network.read', 'Read network and WireGuard status'),
  ('network.manage', 'Manage network interfaces and WireGuard');

INSERT INTO role_permissions(role_id, permission_name)
SELECT 'role_owner', name
FROM permissions
WHERE name IN ('network.read', 'network.manage');
