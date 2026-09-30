package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var (
	ErrUserNotFound = errors.New("user not found")
	ErrRoleNotFound = errors.New("role not found")
)

type PermissionRecord struct {
	Name        string
	Description string
}

type UserProfileUpdate struct {
	DisplayName         string
	PasswordHash        string
	Disabled            bool
	RoleName            string
	Permissions         []string
	ResourcePermissions []ResourcePermissionRecord
}

func (s *Store) ListPermissions(ctx context.Context) ([]PermissionRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT name, description
		FROM permissions
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("list permissions: %w", err)
	}
	defer rows.Close()

	var result []PermissionRecord
	for rows.Next() {
		var item PermissionRecord
		if err := rows.Scan(&item.Name, &item.Description); err != nil {
			return nil, fmt.Errorf("scan permission: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate permissions: %w", err)
	}
	return result, nil
}

func (s *Store) RolePermissions(ctx context.Context, roleName string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT rp.permission_name
		FROM role_permissions rp
		JOIN roles r ON r.id = rp.role_id
		WHERE r.name = ?
		ORDER BY rp.permission_name
	`, strings.TrimSpace(roleName))
	if err != nil {
		return nil, fmt.Errorf("read role permissions: %w", err)
	}
	defer rows.Close()

	var result []string
	for rows.Next() {
		var permission string
		if err := rows.Scan(&permission); err != nil {
			return nil, fmt.Errorf("scan role permission: %w", err)
		}
		result = append(result, permission)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate role permissions: %w", err)
	}
	return result, nil
}

func (s *Store) UserAccount(ctx context.Context, userID string) (UserAccountRecord, error) {
	var user UserRecord
	var disabled int
	var createdAt string
	var lastLoginAt sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT id, username, display_name, password_hash, disabled, created_at, last_login_at
		FROM users
		WHERE id = ?
	`, strings.TrimSpace(userID)).Scan(
		&user.ID,
		&user.Username,
		&user.DisplayName,
		&user.PasswordHash,
		&disabled,
		&createdAt,
		&lastLoginAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return UserAccountRecord{}, ErrUserNotFound
	}
	if err != nil {
		return UserAccountRecord{}, fmt.Errorf("read user account: %w", err)
	}
	user.Disabled = disabled == 1
	user.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return UserAccountRecord{}, fmt.Errorf("parse user created_at: %w", err)
	}
	if lastLoginAt.Valid {
		parsed, parseErr := time.Parse(time.RFC3339Nano, lastLoginAt.String)
		if parseErr != nil {
			return UserAccountRecord{}, fmt.Errorf("parse user last_login_at: %w", parseErr)
		}
		user.LastLoginAt = &parsed
	}

	roles, permissions, resources, err := s.userAccess(ctx, user.ID)
	if err != nil {
		return UserAccountRecord{}, err
	}
	return UserAccountRecord{
		User:                user,
		Roles:               roles,
		Permissions:         permissions,
		ResourcePermissions: resources,
	}, nil
}

func (s *Store) UpdateUserProfile(
	ctx context.Context,
	userID string,
	update UserProfileUpdate,
	now time.Time,
) (UserAccountRecord, error) {
	userID = strings.TrimSpace(userID)
	update.DisplayName = strings.TrimSpace(update.DisplayName)
	update.RoleName = strings.TrimSpace(update.RoleName)
	if userID == "" {
		return UserAccountRecord{}, ErrUserNotFound
	}
	if update.DisplayName == "" {
		return UserAccountRecord{}, errors.New("user display name is required")
	}
	if update.RoleName == "" {
		return UserAccountRecord{}, ErrRoleNotFound
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return UserAccountRecord{}, fmt.Errorf("begin user profile update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var roleID string
	if err := tx.QueryRowContext(ctx, "SELECT id FROM roles WHERE name = ?", update.RoleName).Scan(&roleID); errors.Is(err, sql.ErrNoRows) {
		return UserAccountRecord{}, ErrRoleNotFound
	} else if err != nil {
		return UserAccountRecord{}, fmt.Errorf("read user profile role: %w", err)
	}

	var exists int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE id = ?", userID).Scan(&exists); err != nil {
		return UserAccountRecord{}, fmt.Errorf("validate user: %w", err)
	}
	if exists == 0 {
		return UserAccountRecord{}, ErrUserNotFound
	}

	permissionRows, err := tx.QueryContext(ctx, "SELECT name FROM permissions ORDER BY name")
	if err != nil {
		return UserAccountRecord{}, fmt.Errorf("read permission catalog: %w", err)
	}
	allPermissions := map[string]bool{}
	for permissionRows.Next() {
		var name string
		if err := permissionRows.Scan(&name); err != nil {
			_ = permissionRows.Close()
			return UserAccountRecord{}, fmt.Errorf("scan permission catalog: %w", err)
		}
		allPermissions[name] = true
	}
	if err := permissionRows.Close(); err != nil {
		return UserAccountRecord{}, fmt.Errorf("close permission catalog: %w", err)
	}
	if err := permissionRows.Err(); err != nil {
		return UserAccountRecord{}, fmt.Errorf("iterate permission catalog: %w", err)
	}

	desired := map[string]bool{}
	for _, permission := range update.Permissions {
		permission = strings.TrimSpace(permission)
		if !allPermissions[permission] {
			return UserAccountRecord{}, fmt.Errorf("unknown permission %q", permission)
		}
		desired[permission] = true
	}
	for _, scope := range update.ResourcePermissions {
		if !allPermissions[scope.Permission] {
			return UserAccountRecord{}, fmt.Errorf("unknown scoped permission %q", scope.Permission)
		}
		if strings.TrimSpace(scope.ResourceType) == "" || strings.TrimSpace(scope.ResourceID) == "" {
			return UserAccountRecord{}, errors.New("resource permission requires resource type and id")
		}
	}

	defaultRows, err := tx.QueryContext(ctx, `
		SELECT permission_name
		FROM role_permissions
		WHERE role_id = ?
	`, roleID)
	if err != nil {
		return UserAccountRecord{}, fmt.Errorf("read profile defaults: %w", err)
	}
	defaults := map[string]bool{}
	for defaultRows.Next() {
		var name string
		if err := defaultRows.Scan(&name); err != nil {
			_ = defaultRows.Close()
			return UserAccountRecord{}, fmt.Errorf("scan profile defaults: %w", err)
		}
		defaults[name] = true
	}
	if err := defaultRows.Close(); err != nil {
		return UserAccountRecord{}, fmt.Errorf("close profile defaults: %w", err)
	}
	if err := defaultRows.Err(); err != nil {
		return UserAccountRecord{}, fmt.Errorf("iterate profile defaults: %w", err)
	}

	timestamp := now.UTC().Format(time.RFC3339Nano)
	if update.PasswordHash != "" {
		if _, err := tx.ExecContext(ctx, `
			UPDATE users
			SET display_name = ?, password_hash = ?, disabled = ?, updated_at = ?
			WHERE id = ?
		`, update.DisplayName, update.PasswordHash, boolInt(update.Disabled), timestamp, userID); err != nil {
			return UserAccountRecord{}, fmt.Errorf("update user profile: %w", err)
		}
	} else {
		if _, err := tx.ExecContext(ctx, `
			UPDATE users
			SET display_name = ?, disabled = ?, updated_at = ?
			WHERE id = ?
		`, update.DisplayName, boolInt(update.Disabled), timestamp, userID); err != nil {
			return UserAccountRecord{}, fmt.Errorf("update user profile: %w", err)
		}
	}

	if _, err := tx.ExecContext(ctx, "DELETE FROM user_roles WHERE user_id = ?", userID); err != nil {
		return UserAccountRecord{}, fmt.Errorf("clear user role: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO user_roles(user_id, role_id) VALUES (?, ?)", userID, roleID); err != nil {
		return UserAccountRecord{}, fmt.Errorf("assign user role: %w", err)
	}

	if _, err := tx.ExecContext(ctx, "DELETE FROM user_permission_overrides WHERE user_id = ?", userID); err != nil {
		return UserAccountRecord{}, fmt.Errorf("clear user permission overrides: %w", err)
	}
	if update.RoleName != "administrator" && update.RoleName != "owner" {
		names := make([]string, 0, len(allPermissions))
		for name := range allPermissions {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			if desired[name] == defaults[name] {
				continue
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO user_permission_overrides(user_id, permission_name, allowed)
				VALUES (?, ?, ?)
			`, userID, name, boolInt(desired[name])); err != nil {
				return UserAccountRecord{}, fmt.Errorf("save user permission override: %w", err)
			}
		}
	}

	if _, err := tx.ExecContext(ctx, "DELETE FROM user_resource_permissions WHERE user_id = ?", userID); err != nil {
		return UserAccountRecord{}, fmt.Errorf("clear user resource permissions: %w", err)
	}
	seenScope := map[string]bool{}
	for _, scope := range update.ResourcePermissions {
		scope.Permission = strings.TrimSpace(scope.Permission)
		scope.ResourceType = strings.TrimSpace(scope.ResourceType)
		scope.ResourceID = strings.TrimSpace(scope.ResourceID)
		key := scope.Permission + "\x00" + scope.ResourceType + "\x00" + scope.ResourceID
		if seenScope[key] {
			continue
		}
		seenScope[key] = true
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO user_resource_permissions(user_id, permission_name, resource_type, resource_id)
			VALUES (?, ?, ?, ?)
		`, userID, scope.Permission, scope.ResourceType, scope.ResourceID); err != nil {
			return UserAccountRecord{}, fmt.Errorf("save user resource permission: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return UserAccountRecord{}, fmt.Errorf("commit user profile update: %w", err)
	}
	return s.UserAccount(ctx, userID)
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
