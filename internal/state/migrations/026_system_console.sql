INSERT OR IGNORE INTO permissions(name, description)
VALUES ('system.console', 'Open the server Web console as the Home-AI-Core service user');

INSERT OR IGNORE INTO role_permissions(role_id, permission_name)
VALUES ('role_owner', 'system.console');
