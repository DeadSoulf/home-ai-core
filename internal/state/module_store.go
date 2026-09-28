package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var (
	ErrSigningKeyNotFound  = errors.New("module signing key not found")
	ErrRepositoryNotFound  = errors.New("module repository not found")
)

type ModuleSigningKeyRecord struct {
	KeyID        string    `json:"key_id"`
	Algorithm    string    `json:"algorithm"`
	PublicKeyB64 string    `json:"public_key_b64"`
	CreatedAt    time.Time `json:"created_at"`
}

type ModuleRepositoryRecord struct {
	ID                     string     `json:"id"`
	IndexURL               string     `json:"index_url"`
	SignatureURL           string     `json:"signature_url"`
	KeyID                  string     `json:"key_id"`
	Enabled                bool       `json:"enabled"`
	LastIndexJSON          string     `json:"-"`
	LastIndexSignatureJSON string     `json:"-"`
	LastRefreshedAt        *time.Time `json:"last_refreshed_at,omitempty"`
	LastError              string     `json:"last_error,omitempty"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

func (s *Store) AddModuleSigningKey(ctx context.Context, record ModuleSigningKeyRecord) error {
	createdAt := record.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO module_signing_keys(key_id, algorithm, public_key_b64, created_at)
		VALUES (?, ?, ?, ?)
	`, record.KeyID, record.Algorithm, record.PublicKeyB64, createdAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("add module signing key: %w", err)
	}
	return nil
}

func (s *Store) DeleteModuleSigningKey(ctx context.Context, keyID string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM module_signing_keys WHERE key_id = ?", keyID)
	if err != nil {
		return fmt.Errorf("delete module signing key: %w", err)
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return ErrSigningKeyNotFound
	}
	return nil
}

func (s *Store) ListModuleSigningKeys(ctx context.Context) ([]ModuleSigningKeyRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT key_id, algorithm, public_key_b64, created_at
		FROM module_signing_keys ORDER BY key_id
	`)
	if err != nil {
		return nil, fmt.Errorf("list module signing keys: %w", err)
	}
	defer rows.Close()

	var records []ModuleSigningKeyRecord
	for rows.Next() {
		var record ModuleSigningKeyRecord
		var createdAt string
		if err := rows.Scan(&record.KeyID, &record.Algorithm, &record.PublicKeyB64, &createdAt); err != nil {
			return nil, fmt.Errorf("scan module signing key: %w", err)
		}
		record.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse module signing key created_at: %w", err)
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *Store) UpsertModuleRepository(ctx context.Context, record ModuleRepositoryRecord) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO module_repositories(
			id, index_url, signature_url, key_id, enabled,
			last_index_json, last_index_signature_json, last_refreshed_at, last_error,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			index_url = excluded.index_url,
			signature_url = excluded.signature_url,
			key_id = excluded.key_id,
			enabled = excluded.enabled,
			updated_at = excluded.updated_at
	`,
		record.ID, record.IndexURL, record.SignatureURL, record.KeyID, boolInt(record.Enabled),
		nullIfEmpty(record.LastIndexJSON), nullIfEmpty(record.LastIndexSignatureJSON),
		nullableTime(record.LastRefreshedAt), nullIfEmpty(record.LastError),
		now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("upsert module repository: %w", err)
	}
	return nil
}

func (s *Store) DeleteModuleRepository(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM module_repositories WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete module repository: %w", err)
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return ErrRepositoryNotFound
	}
	return nil
}

func (s *Store) ListModuleRepositories(ctx context.Context) ([]ModuleRepositoryRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, index_url, signature_url, key_id, enabled,
		       COALESCE(last_index_json, ''), COALESCE(last_index_signature_json, ''),
		       last_refreshed_at, COALESCE(last_error, ''), created_at, updated_at
		FROM module_repositories ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("list module repositories: %w", err)
	}
	defer rows.Close()

	var records []ModuleRepositoryRecord
	for rows.Next() {
		record, err := scanModuleRepository(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *Store) SaveModuleRepositorySnapshot(
	ctx context.Context,
	id, indexJSON, signatureJSON, lastError string,
	refreshedAt time.Time,
) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE module_repositories
		SET last_index_json = ?, last_index_signature_json = ?,
		    last_refreshed_at = ?, last_error = ?, updated_at = ?
		WHERE id = ?
	`,
		nullIfEmpty(indexJSON), nullIfEmpty(signatureJSON),
		refreshedAt.UTC().Format(time.RFC3339Nano), nullIfEmpty(lastError),
		refreshedAt.UTC().Format(time.RFC3339Nano), id,
	)
	if err != nil {
		return fmt.Errorf("save module repository snapshot: %w", err)
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return ErrRepositoryNotFound
	}
	return nil
}

type scanner interface {
	Scan(...any) error
}

func scanModuleRepository(row scanner) (ModuleRepositoryRecord, error) {
	var record ModuleRepositoryRecord
	var enabled int
	var lastRefreshed sql.NullString
	var createdAt, updatedAt string
	if err := row.Scan(
		&record.ID, &record.IndexURL, &record.SignatureURL, &record.KeyID, &enabled,
		&record.LastIndexJSON, &record.LastIndexSignatureJSON, &lastRefreshed,
		&record.LastError, &createdAt, &updatedAt,
	); err != nil {
		return ModuleRepositoryRecord{}, fmt.Errorf("scan module repository: %w", err)
	}
	record.Enabled = enabled == 1
	var err error
	record.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return ModuleRepositoryRecord{}, fmt.Errorf("parse module repository created_at: %w", err)
	}
	record.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return ModuleRepositoryRecord{}, fmt.Errorf("parse module repository updated_at: %w", err)
	}
	if lastRefreshed.Valid {
		parsed, err := time.Parse(time.RFC3339Nano, lastRefreshed.String)
		if err != nil {
			return ModuleRepositoryRecord{}, fmt.Errorf("parse module repository last_refreshed_at: %w", err)
		}
		record.LastRefreshedAt = &parsed
	}
	return record, nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}
