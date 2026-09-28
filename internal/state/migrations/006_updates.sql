INSERT INTO permissions(name, description) VALUES
    ('updates.read', 'Check Core update status and available releases'),
    ('updates.manage', 'Install authenticated Home-AI-Core updates');

INSERT INTO role_permissions(role_id, permission_name)
SELECT 'role_owner', name
FROM permissions
WHERE name IN ('updates.read', 'updates.manage');
