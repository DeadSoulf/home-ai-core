package state

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

var (
	ErrNASPoolExists     = errors.New("NAS pool already exists")
	ErrNASPoolNotFound   = errors.New("NAS pool not found")
	ErrNASFolderExists   = errors.New("NAS folder already exists")
	ErrNASFolderNotFound = errors.New("NAS folder not found")
)

type NASPoolRecord struct {
	ID                    string
	Name                  string
	RootPath              string
	StorageDevicePath     string
	StorageFilesystemUUID string
	CreatedBy             string
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type NASFolderRecord struct {
	ID           string
	PoolID       string
	PoolName     string
	PoolRoot     string
	Name         string
	Kind         string
	OwnerUserID  string
	RelativePath string
	CreatedBy    string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (s *Store) CreateNASPool(
	ctx context.Context,
	name, rootPath, storageDevicePath, storageFilesystemUUID, createdBy string,
	now time.Time,
) (NASPoolRecord, error) {
	name = strings.TrimSpace(name)
	rootPath = filepath.Clean(strings.TrimSpace(rootPath))
	storageDevicePath = strings.TrimSpace(storageDevicePath)
	storageFilesystemUUID = strings.TrimSpace(storageFilesystemUUID)
	if name == "" {
		return NASPoolRecord{}, errors.New("NAS pool name is required")
	}
	if rootPath == "." || !filepath.IsAbs(rootPath) {
		return NASPoolRecord{}, errors.New("NAS pool root path must be absolute")
	}

	id, err := newStateID("nsp_")
	if err != nil {
		return NASPoolRecord{}, err
	}
	timestamp := now.UTC().Format(time.RFC3339Nano)

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO nas_pools(
			id, name, root_path, storage_device_path, storage_filesystem_uuid,
			created_by, created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?)
	`, id, name, rootPath, storageDevicePath, storageFilesystemUUID, createdBy, timestamp, timestamp)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return NASPoolRecord{}, ErrNASPoolExists
		}
		return NASPoolRecord{}, fmt.Errorf("create NAS pool: %w", err)
	}

	return NASPoolRecord{
		ID:                    id,
		Name:                  name,
		RootPath:              rootPath,
		StorageDevicePath:     storageDevicePath,
		StorageFilesystemUUID: storageFilesystemUUID,
		CreatedBy:             createdBy,
		CreatedAt:             now.UTC(),
		UpdatedAt:             now.UTC(),
	}, nil
}

func (s *Store) ListNASPools(ctx context.Context) ([]NASPoolRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, root_path, storage_device_path, storage_filesystem_uuid,
		       COALESCE(created_by, ''), created_at, updated_at
		FROM nas_pools
		ORDER BY name COLLATE NOCASE
	`)
	if err != nil {
		return nil, fmt.Errorf("list NAS pools: %w", err)
	}
	defer rows.Close()

	var result []NASPoolRecord
	for rows.Next() {
		var record NASPoolRecord
		var createdAt, updatedAt string
		if err := rows.Scan(
			&record.ID,
			&record.Name,
			&record.RootPath,
			&record.StorageDevicePath,
			&record.StorageFilesystemUUID,
			&record.CreatedBy,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan NAS pool: %w", err)
		}
		record.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse NAS pool created_at: %w", err)
		}
		record.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
		if err != nil {
			return nil, fmt.Errorf("parse NAS pool updated_at: %w", err)
		}
		result = append(result, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate NAS pools: %w", err)
	}
	return result, nil
}

func (s *Store) CreateNASFolder(
	ctx context.Context,
	poolID, name, kind, ownerUserID, createdBy string,
	now time.Time,
) (NASFolderRecord, error) {
	name = strings.TrimSpace(name)
	kind = strings.ToLower(strings.TrimSpace(kind))
	ownerUserID = strings.TrimSpace(ownerUserID)
	if name == "" {
		return NASFolderRecord{}, errors.New("NAS folder name is required")
	}
	if kind != "private" && kind != "shared" {
		return NASFolderRecord{}, errors.New("NAS folder kind must be private or shared")
	}
	if kind == "private" && ownerUserID == "" {
		return NASFolderRecord{}, errors.New("private NAS folder requires an owner user")
	}
	if kind == "shared" {
		ownerUserID = ""
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return NASFolderRecord{}, fmt.Errorf("begin NAS folder creation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var poolName, poolRoot string
	if err := tx.QueryRowContext(
		ctx,
		"SELECT name, root_path FROM nas_pools WHERE id = ?",
		poolID,
	).Scan(&poolName, &poolRoot); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return NASFolderRecord{}, ErrNASPoolNotFound
		}
		return NASFolderRecord{}, fmt.Errorf("read NAS pool: %w", err)
	}

	if kind == "private" {
		var count int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE id = ?", ownerUserID).Scan(&count); err != nil {
			return NASFolderRecord{}, fmt.Errorf("validate NAS folder owner: %w", err)
		}
		if count == 0 {
			return NASFolderRecord{}, errors.New("NAS folder owner user does not exist")
		}
	}

	id, err := newStateID("nsf_")
	if err != nil {
		return NASFolderRecord{}, err
	}
	relativePath := "shared/" + id
	if kind == "private" {
		relativePath = "users/" + ownerUserID + "/" + id
	}
	timestamp := now.UTC().Format(time.RFC3339Nano)

	_, err = tx.ExecContext(ctx, `
		INSERT INTO nas_folders(
			id, pool_id, name, kind, owner_user_id, relative_path, created_by, created_at, updated_at
		)
		VALUES (?, ?, ?, ?, NULLIF(?, ''), ?, NULLIF(?, ''), ?, ?)
	`, id, poolID, name, kind, ownerUserID, relativePath, createdBy, timestamp, timestamp)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return NASFolderRecord{}, ErrNASFolderExists
		}
		return NASFolderRecord{}, fmt.Errorf("create NAS folder: %w", err)
	}

	if kind == "private" {
		for _, permission := range []string{"files.read", "files.write"} {
			if _, err := tx.ExecContext(ctx, `
				INSERT OR IGNORE INTO user_resource_permissions(
					user_id, permission_name, resource_type, resource_id
				)
				VALUES (?, ?, 'file_folder', ?)
			`, ownerUserID, permission, id); err != nil {
				return NASFolderRecord{}, fmt.Errorf("grant private folder permission: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return NASFolderRecord{}, fmt.Errorf("commit NAS folder creation: %w", err)
	}

	return NASFolderRecord{
		ID:           id,
		PoolID:       poolID,
		PoolName:     poolName,
		PoolRoot:     poolRoot,
		Name:         name,
		Kind:         kind,
		OwnerUserID:  ownerUserID,
		RelativePath: relativePath,
		CreatedBy:    createdBy,
		CreatedAt:    now.UTC(),
		UpdatedAt:    now.UTC(),
	}, nil
}

func (s *Store) NASFolder(ctx context.Context, folderID string) (NASFolderRecord, error) {
	var record NASFolderRecord
	var createdAt, updatedAt string
	err := s.db.QueryRowContext(ctx, `
		SELECT f.id, f.pool_id, p.name, p.root_path, f.name, f.kind,
		       COALESCE(f.owner_user_id, ''), f.relative_path,
		       COALESCE(f.created_by, ''), f.created_at, f.updated_at
		FROM nas_folders f
		JOIN nas_pools p ON p.id = f.pool_id
		WHERE f.id = ?
	`, strings.TrimSpace(folderID)).Scan(
		&record.ID,
		&record.PoolID,
		&record.PoolName,
		&record.PoolRoot,
		&record.Name,
		&record.Kind,
		&record.OwnerUserID,
		&record.RelativePath,
		&record.CreatedBy,
		&createdAt,
		&updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return NASFolderRecord{}, ErrNASFolderNotFound
	}
	if err != nil {
		return NASFolderRecord{}, fmt.Errorf("read NAS folder: %w", err)
	}
	record.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return NASFolderRecord{}, fmt.Errorf("parse NAS folder created_at: %w", err)
	}
	record.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return NASFolderRecord{}, fmt.Errorf("parse NAS folder updated_at: %w", err)
	}
	return record, nil
}

func (s *Store) ListNASFolders(ctx context.Context) ([]NASFolderRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT f.id, f.pool_id, p.name, p.root_path, f.name, f.kind,
		       COALESCE(f.owner_user_id, ''), f.relative_path,
		       COALESCE(f.created_by, ''), f.created_at, f.updated_at
		FROM nas_folders f
		JOIN nas_pools p ON p.id = f.pool_id
		ORDER BY f.kind, f.name COLLATE NOCASE
	`)
	if err != nil {
		return nil, fmt.Errorf("list NAS folders: %w", err)
	}
	defer rows.Close()

	var result []NASFolderRecord
	for rows.Next() {
		var record NASFolderRecord
		var createdAt, updatedAt string
		if err := rows.Scan(
			&record.ID,
			&record.PoolID,
			&record.PoolName,
			&record.PoolRoot,
			&record.Name,
			&record.Kind,
			&record.OwnerUserID,
			&record.RelativePath,
			&record.CreatedBy,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan NAS folder: %w", err)
		}
		record.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse NAS folder created_at: %w", err)
		}
		record.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
		if err != nil {
			return nil, fmt.Errorf("parse NAS folder updated_at: %w", err)
		}
		result = append(result, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate NAS folders: %w", err)
	}
	return result, nil
}

func (s *Store) DeleteNASFolder(ctx context.Context, folderID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin NAS folder cleanup: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM user_resource_permissions
		WHERE resource_type = 'file_folder' AND resource_id = ?
	`, folderID); err != nil {
		return fmt.Errorf("remove user NAS folder grants: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM role_resource_permissions
		WHERE resource_type = 'file_folder' AND resource_id = ?
	`, folderID); err != nil {
		return fmt.Errorf("remove role NAS folder grants: %w", err)
	}
	result, err := tx.ExecContext(ctx, "DELETE FROM nas_folders WHERE id = ?", folderID)
	if err != nil {
		return fmt.Errorf("delete NAS folder: %w", err)
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return sql.ErrNoRows
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit NAS folder cleanup: %w", err)
	}
	return nil
}

func newStateID(prefix string) (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate state id: %w", err)
	}
	return prefix + hex.EncodeToString(raw), nil
}
