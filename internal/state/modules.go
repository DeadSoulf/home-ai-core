package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var ErrModuleNotFound = errors.New("module not found")

type ModuleRecord struct {
	ID           string
	Version      string
	Status       string
	ManifestJSON string
	Error        string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (s *Store) UpsertModule(ctx context.Context, record ModuleRecord) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO modules(id, version, status, manifest_json, error_message, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			version = excluded.version,
			manifest_json = excluded.manifest_json,
			updated_at = excluded.updated_at
	`,
		record.ID,
		record.Version,
		record.Status,
		record.ManifestJSON,
		nullIfEmpty(record.Error),
		now,
		now,
	)
	if err != nil {
		return fmt.Errorf("upsert module: %w", err)
	}
	return nil
}

func (s *Store) DeleteModule(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM modules WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete module: %w", err)
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return ErrModuleNotFound
	}
	return nil
}

func (s *Store) SetModuleStatus(ctx context.Context, id, status, errorMessage string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, `
		UPDATE modules
		SET status = ?, error_message = ?, updated_at = ?
		WHERE id = ?
	`, status, nullIfEmpty(errorMessage), now, id)
	if err != nil {
		return fmt.Errorf("set module status: %w", err)
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return ErrModuleNotFound
	}
	return nil
}

func (s *Store) Module(ctx context.Context, id string) (ModuleRecord, error) {
	var record ModuleRecord
	var createdAt, updatedAt string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, version, status, manifest_json, COALESCE(error_message, ''), created_at, updated_at
		FROM modules WHERE id = ?
	`, id).Scan(
		&record.ID,
		&record.Version,
		&record.Status,
		&record.ManifestJSON,
		&record.Error,
		&createdAt,
		&updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ModuleRecord{}, ErrModuleNotFound
	}
	if err != nil {
		return ModuleRecord{}, fmt.Errorf("read module: %w", err)
	}
	record.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return ModuleRecord{}, fmt.Errorf("parse module created_at: %w", err)
	}
	record.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return ModuleRecord{}, fmt.Errorf("parse module updated_at: %w", err)
	}
	return record, nil
}

func (s *Store) ListModules(ctx context.Context) ([]ModuleRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, version, status, manifest_json, COALESCE(error_message, ''), created_at, updated_at
		FROM modules ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("list modules: %w", err)
	}
	defer rows.Close()

	records := []ModuleRecord{}
	for rows.Next() {
		var record ModuleRecord
		var createdAt, updatedAt string
		if err := rows.Scan(
			&record.ID,
			&record.Version,
			&record.Status,
			&record.ManifestJSON,
			&record.Error,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan module: %w", err)
		}
		record.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse module created_at: %w", err)
		}
		record.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
		if err != nil {
			return nil, fmt.Errorf("parse module updated_at: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate modules: %w", err)
	}
	return records, nil
}
