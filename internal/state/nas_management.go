package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type NASFolderAccess struct {
	UserID string `json:"user_id"`
	Read   bool   `json:"read"`
	Write  bool   `json:"write"`
}

func (s *Store) NASSetting(ctx context.Context, name string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM nas_settings WHERE name = ?", name).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return value, err
}
func (s *Store) SetNASSetting(ctx context.Context, name, value string) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO nas_settings(name,value) VALUES (?,?) ON CONFLICT(name) DO UPDATE SET value=excluded.value", name, value)
	return err
}

func (s *Store) NASUserQuotas(ctx context.Context) (map[string]int64, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT user_id, quota_bytes FROM nas_user_quotas")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]int64{}
	for rows.Next() {
		var id string
		var quota int64
		if err := rows.Scan(&id, &quota); err != nil {
			return nil, err
		}
		result[id] = quota
	}
	return result, rows.Err()
}

func (s *Store) SetNASUserQuota(ctx context.Context, id string, quota int64, now time.Time) error {
	if quota < 0 {
		return errors.New("quota must not be negative")
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO nas_user_quotas(user_id, quota_bytes, updated_at)
        VALUES (?, ?, ?) ON CONFLICT(user_id) DO UPDATE SET quota_bytes = excluded.quota_bytes,
        updated_at = excluded.updated_at`, strings.TrimSpace(id), quota, now.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) NASFolderAccess(ctx context.Context, id string) ([]NASFolderAccess, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT user_id,
        MAX(permission_name = 'files.read'), MAX(permission_name = 'files.write')
        FROM user_resource_permissions WHERE resource_type = 'file_folder' AND resource_id = ?
        AND permission_name IN ('files.read','files.write') GROUP BY user_id ORDER BY user_id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []NASFolderAccess{}
	for rows.Next() {
		var item NASFolderAccess
		if err := rows.Scan(&item.UserID, &item.Read, &item.Write); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// UpdateNASFolderManagement keeps identity and the physical path unchanged.
// Only this folder's grants are replaced; unrelated user capabilities survive.
func (s *Store) UpdateNASFolderManagement(ctx context.Context, id, name string, quota, hardQuota int64, access []NASFolderAccess, now time.Time) error {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 128 || strings.ContainsAny(name, "\x00\r\n") {
		return errors.New("folder name must contain 1 to 128 characters")
	}
	if quota < 0 || hardQuota < 0 {
		return errors.New("quota must not be negative")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var owner string
	if err := tx.QueryRowContext(ctx, "SELECT COALESCE(owner_user_id,'') FROM nas_folders WHERE id = ?", id).Scan(&owner); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNASFolderNotFound
		}
		return err
	}
	grants := map[string]NASFolderAccess{}
	for _, item := range access {
		item.UserID = strings.TrimSpace(item.UserID)
		var exists bool
		if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE id = ?)", item.UserID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("unknown folder user %q", item.UserID)
		}
		if item.Write {
			item.Read = true
		}
		grants[item.UserID] = item
	}
	// A private folder continues to belong to its stable identity.
	if owner != "" {
		grants[owner] = NASFolderAccess{UserID: owner, Read: true, Write: true}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE nas_folders SET name = ?, quota_bytes = ?, hard_quota_bytes = ?, updated_at = ? WHERE id = ?`, name, quota, hardQuota, now.UTC().Format(time.RFC3339Nano), id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM user_resource_permissions WHERE resource_type = 'file_folder' AND resource_id = ? AND permission_name IN ('files.read','files.write')", id); err != nil {
		return err
	}
	for _, item := range grants {
		for permission, allowed := range map[string]bool{"files.read": item.Read, "files.write": item.Write} {
			if allowed {
				if _, err := tx.ExecContext(ctx, `INSERT INTO user_resource_permissions(user_id, permission_name, resource_type, resource_id) VALUES (?, ?, 'file_folder', ?)`, item.UserID, permission, id); err != nil {
					return err
				}
			}
		}
	}
	return tx.Commit()
}
