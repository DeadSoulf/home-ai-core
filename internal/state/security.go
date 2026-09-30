package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var (
	ErrAlreadyInitialized = errors.New("security already initialized")
	ErrUserExists         = errors.New("user already exists")
)

type UserRecord struct {
	ID           string
	Username     string
	DisplayName  string
	PasswordHash string
	Disabled     bool
	CreatedAt    time.Time
	LastLoginAt  *time.Time
}

type UserAccountRecord struct {
	User                UserRecord
	Roles               []string
	Permissions         []string
	ResourcePermissions []ResourcePermissionRecord
}

type PermissionRecord struct {
	Name        string
	Description string
}

type ResourcePermissionRecord struct {
	Permission   string
	ResourceType string
	ResourceID   string
}

type SessionRecord struct {
	ID                  string
	User                UserRecord
	TokenHash           string
	CSRFHash            string
	CreatedAt           time.Time
	ExpiresAt           time.Time
	LastSeenAt          time.Time
	Roles               []string
	Permissions         []string
	ResourcePermissions []ResourcePermissionRecord
}

type AuditRecord struct {
	ID            string
	OccurredAt    time.Time
	ActorType     string
	ActorID       string
	Action        string
	TargetType    string
	TargetID      string
	RequestID     string
	CorrelationID string
	Outcome       string
	Metadata      map[string]any
}

func (s *Store) UserCount(ctx context.Context) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return count, nil
}

func (s *Store) CreateOwner(ctx context.Context, id, username, displayName, passwordHash string, now time.Time) (UserRecord, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return UserRecord{}, fmt.Errorf("begin owner creation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var count int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		return UserRecord{}, fmt.Errorf("count users during bootstrap: %w", err)
	}
	if count != 0 {
		return UserRecord{}, ErrAlreadyInitialized
	}

	timestamp := now.UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO users(id, username, display_name, password_hash, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, id, username, displayName, passwordHash, timestamp, timestamp); err != nil {
		return UserRecord{}, fmt.Errorf("insert owner: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO user_roles(user_id, role_id) VALUES (?, 'role_owner')", id); err != nil {
		return UserRecord{}, fmt.Errorf("assign owner role: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return UserRecord{}, fmt.Errorf("commit owner creation: %w", err)
	}

	return UserRecord{ID: id, Username: username, DisplayName: displayName, PasswordHash: passwordHash, CreatedAt: now.UTC()}, nil
}

func (s *Store) CreateUser(
	ctx context.Context,
	id, username, displayName, passwordHash, roleID string,
	now time.Time,
) (UserRecord, error) {
	return s.CreateUserWithAccess(
		ctx,
		id,
		username,
		displayName,
		passwordHash,
		roleID,
		nil,
		nil,
		now,
	)
}

func (s *Store) CreateUserWithAccess(
	ctx context.Context,
	id, username, displayName, passwordHash, roleID string,
	permissions []string,
	resourcePermissions []ResourcePermissionRecord,
	now time.Time,
) (UserRecord, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return UserRecord{}, fmt.Errorf("begin user creation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var count int
	if err := tx.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM users WHERE username = ? COLLATE NOCASE",
		username,
	).Scan(&count); err != nil {
		return UserRecord{}, fmt.Errorf("check existing user: %w", err)
	}
	if count != 0 {
		return UserRecord{}, ErrUserExists
	}

	timestamp := now.UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO users(id, username, display_name, password_hash, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, id, username, displayName, passwordHash, timestamp, timestamp); err != nil {
		return UserRecord{}, fmt.Errorf("insert user: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO user_roles(user_id, role_id) VALUES (?, ?)", id, roleID); err != nil {
		return UserRecord{}, fmt.Errorf("assign user role: %w", err)
	}
	for _, permission := range permissions {
		if _, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO user_permissions(user_id, permission_name)
			VALUES (?, ?)
		`, id, permission); err != nil {
			return UserRecord{}, fmt.Errorf("assign user permission %s: %w", permission, err)
		}
	}
	for _, scope := range resourcePermissions {
		if _, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO user_resource_permissions(user_id, permission_name, resource_type, resource_id)
			VALUES (?, ?, ?, ?)
		`, id, scope.Permission, scope.ResourceType, scope.ResourceID); err != nil {
			return UserRecord{}, fmt.Errorf("assign user resource permission %s: %w", scope.Permission, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return UserRecord{}, fmt.Errorf("commit user creation: %w", err)
	}

	return UserRecord{
		ID:           id,
		Username:     username,
		DisplayName:  displayName,
		PasswordHash: passwordHash,
		CreatedAt:    now.UTC(),
	}, nil
}

func (s *Store) ListUsers(ctx context.Context) ([]UserAccountRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, username, display_name, password_hash, disabled, created_at, last_login_at
		FROM users
		ORDER BY username COLLATE NOCASE
	`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}

	var users []UserRecord
	for rows.Next() {
		var user UserRecord
		var disabled int
		var createdAt string
		var lastLoginAt sql.NullString
		if err := rows.Scan(
			&user.ID,
			&user.Username,
			&user.DisplayName,
			&user.PasswordHash,
			&disabled,
			&createdAt,
			&lastLoginAt,
		); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan user: %w", err)
		}
		user.Disabled = disabled == 1
		user.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("parse user created_at: %w", err)
		}
		if lastLoginAt.Valid {
			parsed, err := time.Parse(time.RFC3339Nano, lastLoginAt.String)
			if err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("parse user last_login_at: %w", err)
			}
			user.LastLoginAt = &parsed
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate users: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close user rows: %w", err)
	}

	result := make([]UserAccountRecord, 0, len(users))
	for _, user := range users {
		roles, permissions, resourcePermissions, err := s.userAccess(ctx, user.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, UserAccountRecord{
			User:                user,
			Roles:               roles,
			Permissions:         permissions,
			ResourcePermissions: resourcePermissions,
		})
	}
	return result, nil
}

func (s *Store) userRoles(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.name
		FROM roles r
		JOIN user_roles ur ON ur.role_id = r.id
		WHERE ur.user_id = ?
		ORDER BY r.name
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("read user roles: %w", err)
	}
	defer rows.Close()

	var roles []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan user role: %w", err)
		}
		roles = append(roles, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user roles: %w", err)
	}
	return roles, nil
}

func (s *Store) UserByUsername(ctx context.Context, username string) (UserRecord, error) {
	var user UserRecord
	var disabled int
	var createdAt string
	var lastLoginAt sql.NullString

	err := s.db.QueryRowContext(ctx, `
		SELECT id, username, display_name, password_hash, disabled, created_at, last_login_at
		FROM users
		WHERE username = ? COLLATE NOCASE
	`, username).Scan(&user.ID, &user.Username, &user.DisplayName, &user.PasswordHash, &disabled, &createdAt, &lastLoginAt)
	if err != nil {
		return UserRecord{}, err
	}

	user.Disabled = disabled == 1
	user.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return UserRecord{}, fmt.Errorf("parse user created_at: %w", err)
	}
	if lastLoginAt.Valid {
		parsed, err := time.Parse(time.RFC3339Nano, lastLoginAt.String)
		if err != nil {
			return UserRecord{}, fmt.Errorf("parse user last_login_at: %w", err)
		}
		user.LastLoginAt = &parsed
	}
	return user, nil
}

func (s *Store) UpdateLastLogin(ctx context.Context, userID string, at time.Time) error {
	timestamp := at.UTC().Format(time.RFC3339Nano)
	if _, err := s.db.ExecContext(ctx, "UPDATE users SET last_login_at = ?, updated_at = ? WHERE id = ?", timestamp, timestamp, userID); err != nil {
		return fmt.Errorf("update last login: %w", err)
	}
	return nil
}

func (s *Store) CreateSession(ctx context.Context, id, userID, tokenHash, csrfHash string, createdAt, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions(id, user_id, token_hash, csrf_hash, created_at, expires_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, id, userID, tokenHash, csrfHash,
		createdAt.UTC().Format(time.RFC3339Nano),
		expiresAt.UTC().Format(time.RFC3339Nano),
		createdAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

func (s *Store) SessionByTokenHash(ctx context.Context, tokenHash string, now time.Time) (SessionRecord, error) {
	var record SessionRecord
	var disabled int
	var createdAt, expiresAt, lastSeenAt, userCreatedAt string
	var lastLoginAt sql.NullString

	err := s.db.QueryRowContext(ctx, `
		SELECT s.id, s.token_hash, s.csrf_hash, s.created_at, s.expires_at, s.last_seen_at,
		       u.id, u.username, u.display_name, u.password_hash, u.disabled, u.created_at, u.last_login_at
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = ? AND s.revoked_at IS NULL AND s.expires_at > ? AND u.disabled = 0
	`, tokenHash, now.UTC().Format(time.RFC3339Nano)).Scan(
		&record.ID, &record.TokenHash, &record.CSRFHash, &createdAt, &expiresAt, &lastSeenAt,
		&record.User.ID, &record.User.Username, &record.User.DisplayName, &record.User.PasswordHash,
		&disabled, &userCreatedAt, &lastLoginAt)
	if err != nil {
		return SessionRecord{}, err
	}

	record.User.Disabled = disabled == 1
	if record.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return SessionRecord{}, fmt.Errorf("parse session created_at: %w", err)
	}
	if record.ExpiresAt, err = time.Parse(time.RFC3339Nano, expiresAt); err != nil {
		return SessionRecord{}, fmt.Errorf("parse session expires_at: %w", err)
	}
	if record.LastSeenAt, err = time.Parse(time.RFC3339Nano, lastSeenAt); err != nil {
		return SessionRecord{}, fmt.Errorf("parse session last_seen_at: %w", err)
	}
	if record.User.CreatedAt, err = time.Parse(time.RFC3339Nano, userCreatedAt); err != nil {
		return SessionRecord{}, fmt.Errorf("parse session user created_at: %w", err)
	}
	if lastLoginAt.Valid {
		parsed, err := time.Parse(time.RFC3339Nano, lastLoginAt.String)
		if err != nil {
			return SessionRecord{}, fmt.Errorf("parse session user last_login_at: %w", err)
		}
		record.User.LastLoginAt = &parsed
	}

	record.Roles, record.Permissions, record.ResourcePermissions, err = s.userAccess(ctx, record.User.ID)
	if err != nil {
		return SessionRecord{}, err
	}
	return record, nil
}

func (s *Store) userAccess(ctx context.Context, userID string) ([]string, []string, []ResourcePermissionRecord, error) {
	roleRows, err := s.db.QueryContext(ctx, `
		SELECT r.name FROM roles r
		JOIN user_roles ur ON ur.role_id = r.id
		WHERE ur.user_id = ? ORDER BY r.name
	`, userID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read user roles: %w", err)
	}
	var roles []string
	for roleRows.Next() {
		var name string
		if err := roleRows.Scan(&name); err != nil {
			_ = roleRows.Close()
			return nil, nil, nil, fmt.Errorf("scan user role: %w", err)
		}
		roles = append(roles, name)
	}
	if err := roleRows.Close(); err != nil {
		return nil, nil, nil, fmt.Errorf("close role rows: %w", err)
	}

	permissionRows, err := s.db.QueryContext(ctx, `
		SELECT permission_name
		FROM (
			SELECT rp.permission_name AS permission_name
			FROM role_permissions rp
			JOIN user_roles ur ON ur.role_id = rp.role_id
			WHERE ur.user_id = ?
			UNION
			SELECT up.permission_name AS permission_name
			FROM user_permissions up
			WHERE up.user_id = ?
		)
		ORDER BY permission_name
	`, userID, userID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read user permissions: %w", err)
	}

	var permissions []string
	for permissionRows.Next() {
		var name string
		if err := permissionRows.Scan(&name); err != nil {
			_ = permissionRows.Close()
			return nil, nil, nil, fmt.Errorf("scan user permission: %w", err)
		}
		permissions = append(permissions, name)
	}
	if err := permissionRows.Close(); err != nil {
		return nil, nil, nil, fmt.Errorf("close permission rows: %w", err)
	}
	if err := permissionRows.Err(); err != nil {
		return nil, nil, nil, fmt.Errorf("iterate user permissions: %w", err)
	}

	scopeRows, err := s.db.QueryContext(ctx, `
		SELECT permission_name, resource_type, resource_id
		FROM (
			SELECT rrp.permission_name AS permission_name,
			       rrp.resource_type AS resource_type,
			       rrp.resource_id AS resource_id
			FROM role_resource_permissions rrp
			JOIN user_roles ur ON ur.role_id = rrp.role_id
			WHERE ur.user_id = ?
			UNION
			SELECT urp.permission_name AS permission_name,
			       urp.resource_type AS resource_type,
			       urp.resource_id AS resource_id
			FROM user_resource_permissions urp
			WHERE urp.user_id = ?
		)
		ORDER BY permission_name, resource_type, resource_id
	`, userID, userID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read user resource permissions: %w", err)
	}
	defer scopeRows.Close()

	var resourcePermissions []ResourcePermissionRecord
	for scopeRows.Next() {
		var item ResourcePermissionRecord
		if err := scopeRows.Scan(&item.Permission, &item.ResourceType, &item.ResourceID); err != nil {
			return nil, nil, nil, fmt.Errorf("scan user resource permission: %w", err)
		}
		resourcePermissions = append(resourcePermissions, item)
	}
	if err := scopeRows.Err(); err != nil {
		return nil, nil, nil, fmt.Errorf("iterate user resource permissions: %w", err)
	}

	return roles, permissions, resourcePermissions, nil
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
		var record PermissionRecord
		if err := rows.Scan(&record.Name, &record.Description); err != nil {
			return nil, fmt.Errorf("scan permission: %w", err)
		}
		result = append(result, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate permissions: %w", err)
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
	`, userID).Scan(
		&user.ID,
		&user.Username,
		&user.DisplayName,
		&user.PasswordHash,
		&disabled,
		&createdAt,
		&lastLoginAt,
	)
	if err != nil {
		return UserAccountRecord{}, err
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
	roles, permissions, resourcePermissions, err := s.userAccess(ctx, userID)
	if err != nil {
		return UserAccountRecord{}, err
	}
	return UserAccountRecord{
		User:                user,
		Roles:               roles,
		Permissions:         permissions,
		ResourcePermissions: resourcePermissions,
	}, nil
}

func (s *Store) SetUserAccess(
	ctx context.Context,
	userID, roleID string,
	permissions []string,
	resourcePermissions []ResourcePermissionRecord,
	disabled bool,
	now time.Time,
) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin user access update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, `
		UPDATE users
		SET disabled = ?, updated_at = ?
		WHERE id = ?
	`, boolInt(disabled), now.UTC().Format(time.RFC3339Nano), userID)
	if err != nil {
		return fmt.Errorf("update user account state: %w", err)
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return sql.ErrNoRows
	}

	if _, err := tx.ExecContext(ctx, "DELETE FROM user_roles WHERE user_id = ?", userID); err != nil {
		return fmt.Errorf("clear user roles: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO user_roles(user_id, role_id) VALUES (?, ?)", userID, roleID); err != nil {
		return fmt.Errorf("assign user role: %w", err)
	}

	if _, err := tx.ExecContext(ctx, "DELETE FROM user_permissions WHERE user_id = ?", userID); err != nil {
		return fmt.Errorf("clear user permissions: %w", err)
	}
	for _, permission := range permissions {
		if _, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO user_permissions(user_id, permission_name)
			VALUES (?, ?)
		`, userID, permission); err != nil {
			return fmt.Errorf("assign user permission %s: %w", permission, err)
		}
	}

	if _, err := tx.ExecContext(ctx, "DELETE FROM user_resource_permissions WHERE user_id = ?", userID); err != nil {
		return fmt.Errorf("clear user resource permissions: %w", err)
	}
	for _, scope := range resourcePermissions {
		if _, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO user_resource_permissions(user_id, permission_name, resource_type, resource_id)
			VALUES (?, ?, ?, ?)
		`, userID, scope.Permission, scope.ResourceType, scope.ResourceID); err != nil {
			return fmt.Errorf("assign user resource permission %s: %w", scope.Permission, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit user access update: %w", err)
	}
	return nil
}

func (s *Store) CountEnabledUsersWithRole(ctx context.Context, roleID string) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM users u
		JOIN user_roles ur ON ur.user_id = u.id
		WHERE ur.role_id = ? AND u.disabled = 0
	`, roleID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count enabled users with role: %w", err)
	}
	return count, nil
}

func (s *Store) GrantRoleResourcePermission(
	ctx context.Context,
	roleID, permission, resourceType, resourceID string,
) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO role_resource_permissions(role_id, permission_name, resource_type, resource_id)
		VALUES (?, ?, ?, ?)
	`, roleID, permission, resourceType, resourceID)
	if err != nil {
		return fmt.Errorf("grant role resource permission: %w", err)
	}
	return nil
}

func (s *Store) GrantUserResourcePermission(
	ctx context.Context,
	userID, permission, resourceType, resourceID string,
) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO user_resource_permissions(user_id, permission_name, resource_type, resource_id)
		VALUES (?, ?, ?, ?)
	`, userID, permission, resourceType, resourceID)
	if err != nil {
		return fmt.Errorf("grant user resource permission: %w", err)
	}
	return nil
}

func (s *Store) RevokeSession(ctx context.Context, sessionID string, at time.Time) error {
	result, err := s.db.ExecContext(ctx, "UPDATE sessions SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL", at.UTC().Format(time.RFC3339Nano), sessionID)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) CleanupExpiredSessions(ctx context.Context, now time.Time) error {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at <= ? OR revoked_at IS NOT NULL", now.UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("cleanup sessions: %w", err)
	}
	return nil
}

func (s *Store) WriteAudit(ctx context.Context, record AuditRecord) error {
	metadata := record.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("encode audit metadata: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO audit_events(id, occurred_at, actor_type, actor_id, action, target_type, target_id,
			request_id, correlation_id, outcome, metadata_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, record.ID, record.OccurredAt.UTC().Format(time.RFC3339Nano), record.ActorType,
		nullIfEmpty(record.ActorID), record.Action, nullIfEmpty(record.TargetType), nullIfEmpty(record.TargetID),
		nullIfEmpty(record.RequestID), nullIfEmpty(record.CorrelationID), record.Outcome, string(raw))
	if err != nil {
		return fmt.Errorf("write audit event: %w", err)
	}
	return nil
}

func (s *Store) ListAudit(ctx context.Context, limit int) ([]AuditRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, occurred_at, actor_type, COALESCE(actor_id, ''), action,
		       COALESCE(target_type, ''), COALESCE(target_id, ''), COALESCE(request_id, ''),
		       COALESCE(correlation_id, ''), outcome, metadata_json
		FROM audit_events ORDER BY occurred_at DESC LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list audit events: %w", err)
	}
	defer rows.Close()

	records := make([]AuditRecord, 0)
	for rows.Next() {
		var record AuditRecord
		var occurredAt, metadataJSON string
		if err := rows.Scan(&record.ID, &occurredAt, &record.ActorType, &record.ActorID, &record.Action,
			&record.TargetType, &record.TargetID, &record.RequestID, &record.CorrelationID, &record.Outcome, &metadataJSON); err != nil {
			return nil, fmt.Errorf("scan audit event: %w", err)
		}
		record.OccurredAt, err = time.Parse(time.RFC3339Nano, occurredAt)
		if err != nil {
			return nil, fmt.Errorf("parse audit occurred_at: %w", err)
		}
		if err := json.Unmarshal([]byte(metadataJSON), &record.Metadata); err != nil {
			return nil, fmt.Errorf("decode audit metadata: %w", err)
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}
