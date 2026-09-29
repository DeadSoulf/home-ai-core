CREATE TABLE role_resource_permissions (
    role_id TEXT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_name TEXT NOT NULL REFERENCES permissions(name) ON DELETE CASCADE,
    resource_type TEXT NOT NULL,
    resource_id TEXT NOT NULL,
    PRIMARY KEY (role_id, permission_name, resource_type, resource_id)
);

CREATE INDEX idx_role_resource_permissions_lookup
ON role_resource_permissions(permission_name, resource_type, resource_id);

CREATE TABLE user_resource_permissions (
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    permission_name TEXT NOT NULL REFERENCES permissions(name) ON DELETE CASCADE,
    resource_type TEXT NOT NULL,
    resource_id TEXT NOT NULL,
    PRIMARY KEY (user_id, permission_name, resource_type, resource_id)
);

CREATE INDEX idx_user_resource_permissions_lookup
ON user_resource_permissions(permission_name, resource_type, resource_id);
