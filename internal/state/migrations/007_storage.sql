INSERT INTO permissions(name, description) VALUES
    ('storage.read', 'Read block device and filesystem information'),
    ('storage.manage', 'Mount, unmount and format non-system block devices');

INSERT INTO role_permissions(role_id, permission_name)
SELECT 'role_owner', name
FROM permissions
WHERE name IN ('storage.read', 'storage.manage');
