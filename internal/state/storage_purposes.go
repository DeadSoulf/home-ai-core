package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	StoragePurposeFiles = "files"
	StoragePurposeVideo = "video"
)

type StoragePurposeRecord struct {
	DevicePath     string
	FilesystemUUID string
	Purpose        string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func NormalizeStoragePurpose(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case StoragePurposeFiles:
		return StoragePurposeFiles, nil
	case StoragePurposeVideo:
		return StoragePurposeVideo, nil
	default:
		return "", errors.New("storage purpose must be files or video")
	}
}

func (s *Store) SetStoragePurpose(
	ctx context.Context,
	devicePath, filesystemUUID, purpose string,
	now time.Time,
) (StoragePurposeRecord, error) {
	devicePath = strings.TrimSpace(devicePath)
	filesystemUUID = strings.TrimSpace(filesystemUUID)
	if devicePath == "" {
		return StoragePurposeRecord{}, errors.New("storage device path is required")
	}
	purpose, err := NormalizeStoragePurpose(purpose)
	if err != nil {
		return StoragePurposeRecord{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return StoragePurposeRecord{}, fmt.Errorf("begin storage purpose update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	timestamp := now.UTC().Format(time.RFC3339Nano)
	if filesystemUUID != "" {
		result, err := tx.ExecContext(ctx, `
			UPDATE storage_purposes
			SET device_path = ?, purpose = ?, updated_at = ?
			WHERE filesystem_uuid = ?
		`, devicePath, purpose, timestamp, filesystemUUID)
		if err != nil {
			return StoragePurposeRecord{}, fmt.Errorf("update storage purpose by filesystem UUID: %w", err)
		}
		if count, _ := result.RowsAffected(); count > 0 {
			if err := tx.Commit(); err != nil {
				return StoragePurposeRecord{}, fmt.Errorf("commit storage purpose update: %w", err)
			}
			return s.storagePurposeByDevicePath(ctx, devicePath)
		}
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO storage_purposes(device_path, filesystem_uuid, purpose, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(device_path) DO UPDATE SET
			filesystem_uuid = excluded.filesystem_uuid,
			purpose = excluded.purpose,
			updated_at = excluded.updated_at
	`, devicePath, filesystemUUID, purpose, timestamp, timestamp)
	if err != nil {
		return StoragePurposeRecord{}, fmt.Errorf("set storage purpose: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return StoragePurposeRecord{}, fmt.Errorf("commit storage purpose update: %w", err)
	}
	return s.storagePurposeByDevicePath(ctx, devicePath)
}

func (s *Store) ClearStoragePurpose(ctx context.Context, devicePath, filesystemUUID string) error {
	devicePath = strings.TrimSpace(devicePath)
	filesystemUUID = strings.TrimSpace(filesystemUUID)
	if devicePath == "" && filesystemUUID == "" {
		return errors.New("storage device path or filesystem UUID is required")
	}

	var (
		result sql.Result
		err    error
	)
	if filesystemUUID != "" {
		result, err = s.db.ExecContext(ctx, `
			DELETE FROM storage_purposes
			WHERE device_path = ? OR filesystem_uuid = ?
		`, devicePath, filesystemUUID)
	} else {
		result, err = s.db.ExecContext(ctx, "DELETE FROM storage_purposes WHERE device_path = ?", devicePath)
	}
	if err != nil {
		return fmt.Errorf("clear storage purpose: %w", err)
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return nil
	}
	return nil
}

func (s *Store) ListStoragePurposes(ctx context.Context) ([]StoragePurposeRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT device_path, filesystem_uuid, purpose, created_at, updated_at
		FROM storage_purposes
		ORDER BY device_path
	`)
	if err != nil {
		return nil, fmt.Errorf("list storage purposes: %w", err)
	}
	defer rows.Close()

	var result []StoragePurposeRecord
	for rows.Next() {
		record, err := scanStoragePurpose(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate storage purposes: %w", err)
	}
	return result, nil
}

func (s *Store) storagePurposeByDevicePath(ctx context.Context, devicePath string) (StoragePurposeRecord, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT device_path, filesystem_uuid, purpose, created_at, updated_at
		FROM storage_purposes
		WHERE device_path = ?
	`, devicePath)
	return scanStoragePurpose(row)
}

type storagePurposeScanner interface {
	Scan(dest ...any) error
}

func scanStoragePurpose(scanner storagePurposeScanner) (StoragePurposeRecord, error) {
	var record StoragePurposeRecord
	var createdAt, updatedAt string
	if err := scanner.Scan(
		&record.DevicePath,
		&record.FilesystemUUID,
		&record.Purpose,
		&createdAt,
		&updatedAt,
	); err != nil {
		return StoragePurposeRecord{}, fmt.Errorf("scan storage purpose: %w", err)
	}
	var err error
	record.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return StoragePurposeRecord{}, fmt.Errorf("parse storage purpose created_at: %w", err)
	}
	record.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return StoragePurposeRecord{}, fmt.Errorf("parse storage purpose updated_at: %w", err)
	}
	return record, nil
}
