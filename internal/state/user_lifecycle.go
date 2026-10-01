package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrUserCredentialChanged = errors.New("user credentials changed; authenticate again")
var ErrLastEnabledAdministrator = errors.New("at least one enabled administrator must remain")

func (s *Store) UpdateUserIdentity(ctx context.Context, id, username, displayName string, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var oldUsername string
	if err := tx.QueryRowContext(ctx, "SELECT username FROM users WHERE id = ?", id).Scan(&oldUsername); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "UPDATE users SET username = ?, display_name = ?, updated_at = ? WHERE id = ?", username, displayName, now.UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return ErrUserExists
		}
		return fmt.Errorf("update user identity: %w", err)
	}
	if oldUsername != username {
		if _, err := tx.ExecContext(ctx, "UPDATE sessions SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL", now.UTC().Format(time.RFC3339Nano), id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ChangeUserPassword binds the write to the verified previous hash and revokes
// every session in the same transaction. A simultaneous reset cannot revive it.
func (s *Store) ChangeUserPassword(ctx context.Context, id, previousHash, newHash string, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, "UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ? AND password_hash = ?", newHash, now.UTC().Format(time.RFC3339Nano), id, previousHash)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return ErrUserCredentialChanged
	}
	if _, err := tx.ExecContext(ctx, "UPDATE sessions SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL", now.UTC().Format(time.RFC3339Nano), id); err != nil {
		return err
	}
	return tx.Commit()
}

// Protect the last administrator inside the same transaction as access changes.
func checkEnabledAdministrator(ctx context.Context, tx *sql.Tx, id, role string, disabled bool) error {
	if role == "role_owner" && !disabled {
		return nil
	}
	var isOwner, others int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM users u JOIN user_roles r ON r.user_id = u.id WHERE u.id = ? AND u.disabled = 0 AND r.role_id = 'role_owner'", id).Scan(&isOwner); err != nil {
		return err
	}
	if isOwner == 0 {
		return nil
	}
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM users u JOIN user_roles r ON r.user_id = u.id WHERE u.id <> ? AND u.disabled = 0 AND r.role_id = 'role_owner'", id).Scan(&others); err != nil {
		return err
	}
	if others == 0 {
		return ErrLastEnabledAdministrator
	}
	return nil
}
