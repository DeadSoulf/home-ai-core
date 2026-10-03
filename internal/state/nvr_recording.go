package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrNVRStorageTargetNotFound = errors.New("NVR storage target not found")

type NVRStorageTargetRecord struct {
	ID             string
	DevicePath     string
	FilesystemUUID string
	Mountpoint     string
	ReservePercent int
	Active         bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type NVRRecordingSegmentRecord struct {
	ID              string
	CameraID        string
	StorageTargetID string
	StartAt         time.Time
	EndAt           time.Time
	RelativePath    string
	Codec           string
	Width           int
	Height          int
	SizeBytes       int64
	Protected       bool
	Status          string
	CreatedAt       time.Time
}

func (s *Store) SetNVRStorageTarget(
	ctx context.Context,
	devicePath, filesystemUUID, mountpoint string,
	reservePercent int,
	active bool,
	now time.Time,
) (NVRStorageTargetRecord, error) {
	devicePath = strings.TrimSpace(devicePath)
	filesystemUUID = strings.TrimSpace(filesystemUUID)
	mountpoint = strings.TrimSpace(mountpoint)
	if devicePath == "" {
		return NVRStorageTargetRecord{}, errors.New("NVR storage device path is required")
	}
	if mountpoint == "" {
		return NVRStorageTargetRecord{}, errors.New("NVR storage mountpoint is required")
	}
	if reservePercent < 1 || reservePercent > 50 {
		return NVRStorageTargetRecord{}, errors.New("NVR storage reserve percent must be between 1 and 50")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return NVRStorageTargetRecord{}, fmt.Errorf("begin NVR storage target update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var id string
	err = tx.QueryRowContext(ctx, `
		SELECT id
		FROM nvr_storage_targets
		WHERE (filesystem_uuid <> '' AND filesystem_uuid = ?)
		   OR (filesystem_uuid = '' AND device_path = ?)
		ORDER BY created_at
		LIMIT 1
	`, filesystemUUID, devicePath).Scan(&id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		id, err = newStateID("nvt_")
		if err != nil {
			return NVRStorageTargetRecord{}, err
		}
		timestamp := now.UTC().Format(time.RFC3339Nano)
		if active {
			if _, err := tx.ExecContext(ctx, "UPDATE nvr_storage_targets SET active = 0, updated_at = ?", timestamp); err != nil {
				return NVRStorageTargetRecord{}, fmt.Errorf("deactivate NVR storage targets: %w", err)
			}
		}
		activeValue := 0
		if active {
			activeValue = 1
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO nvr_storage_targets(
				id, device_path, filesystem_uuid, mountpoint, reserve_percent, active, created_at, updated_at
			)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, id, devicePath, filesystemUUID, mountpoint, reservePercent, activeValue, timestamp, timestamp); err != nil {
			return NVRStorageTargetRecord{}, fmt.Errorf("create NVR storage target: %w", err)
		}
	case err != nil:
		return NVRStorageTargetRecord{}, fmt.Errorf("find NVR storage target: %w", err)
	default:
		timestamp := now.UTC().Format(time.RFC3339Nano)
		if active {
			if _, err := tx.ExecContext(ctx, "UPDATE nvr_storage_targets SET active = 0, updated_at = ? WHERE id <> ?", timestamp, id); err != nil {
				return NVRStorageTargetRecord{}, fmt.Errorf("deactivate NVR storage targets: %w", err)
			}
		}
		activeValue := 0
		if active {
			activeValue = 1
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE nvr_storage_targets
			SET device_path = ?, filesystem_uuid = ?, mountpoint = ?, reserve_percent = ?, active = ?, updated_at = ?
			WHERE id = ?
		`, devicePath, filesystemUUID, mountpoint, reservePercent, activeValue, timestamp, id); err != nil {
			return NVRStorageTargetRecord{}, fmt.Errorf("update NVR storage target: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return NVRStorageTargetRecord{}, fmt.Errorf("commit NVR storage target update: %w", err)
	}
	return s.NVRStorageTarget(ctx, id)
}

func (s *Store) NVRStorageTarget(ctx context.Context, id string) (NVRStorageTargetRecord, error) {
	var record NVRStorageTargetRecord
	var active int
	var createdAt, updatedAt string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, device_path, filesystem_uuid, mountpoint, reserve_percent, active, created_at, updated_at
		FROM nvr_storage_targets
		WHERE id = ?
	`, strings.TrimSpace(id)).Scan(
		&record.ID, &record.DevicePath, &record.FilesystemUUID, &record.Mountpoint,
		&record.ReservePercent, &active, &createdAt, &updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return NVRStorageTargetRecord{}, ErrNVRStorageTargetNotFound
	}
	if err != nil {
		return NVRStorageTargetRecord{}, fmt.Errorf("read NVR storage target: %w", err)
	}
	record.Active = active == 1
	record.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return NVRStorageTargetRecord{}, fmt.Errorf("parse NVR storage target created_at: %w", err)
	}
	record.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return NVRStorageTargetRecord{}, fmt.Errorf("parse NVR storage target updated_at: %w", err)
	}
	return record, nil
}

func (s *Store) ActiveNVRStorageTarget(ctx context.Context) (NVRStorageTargetRecord, error) {
	var id string
	err := s.db.QueryRowContext(ctx, "SELECT id FROM nvr_storage_targets WHERE active = 1 LIMIT 1").Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return NVRStorageTargetRecord{}, ErrNVRStorageTargetNotFound
	}
	if err != nil {
		return NVRStorageTargetRecord{}, fmt.Errorf("read active NVR storage target: %w", err)
	}
	return s.NVRStorageTarget(ctx, id)
}

func (s *Store) CreateNVRRecordingSegment(
	ctx context.Context,
	cameraID, storageTargetID string,
	startAt, endAt time.Time,
	relativePath, codec string,
	width, height int,
	sizeBytes int64,
	protected bool,
	status string,
	now time.Time,
) (NVRRecordingSegmentRecord, error) {
	cameraID = strings.TrimSpace(cameraID)
	storageTargetID = strings.TrimSpace(storageTargetID)
	relativePath = strings.TrimSpace(relativePath)
	codec = strings.TrimSpace(codec)
	status = strings.ToLower(strings.TrimSpace(status))
	if cameraID == "" || storageTargetID == "" || relativePath == "" {
		return NVRRecordingSegmentRecord{}, errors.New("camera, storage target and segment path are required")
	}
	if endAt.Before(startAt) {
		return NVRRecordingSegmentRecord{}, errors.New("segment end time cannot be before start time")
	}
	if width < 0 || height < 0 || sizeBytes < 0 {
		return NVRRecordingSegmentRecord{}, errors.New("segment metadata cannot be negative")
	}
	switch status {
	case "writing", "complete", "recovering", "corrupt", "missing":
	default:
		return NVRRecordingSegmentRecord{}, errors.New("NVR segment status is invalid")
	}
	id, err := newStateID("nvs_")
	if err != nil {
		return NVRRecordingSegmentRecord{}, err
	}
	protectedValue := 0
	if protected {
		protectedValue = 1
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO nvr_recording_segments(
			id, camera_id, storage_target_id, start_at, end_at, relative_path, codec,
			width, height, size_bytes, protected, status, created_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, cameraID, storageTargetID,
		startAt.UTC().Format(time.RFC3339Nano), endAt.UTC().Format(time.RFC3339Nano),
		relativePath, codec, width, height, sizeBytes, protectedValue, status,
		now.UTC().Format(time.RFC3339Nano),
	); err != nil {
		return NVRRecordingSegmentRecord{}, fmt.Errorf("create NVR recording segment: %w", err)
	}
	return s.NVRRecordingSegment(ctx, id)
}

func (s *Store) NVRRecordingSegment(ctx context.Context, id string) (NVRRecordingSegmentRecord, error) {
	var record NVRRecordingSegmentRecord
	var protected int
	var startAt, endAt, createdAt string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, camera_id, storage_target_id, start_at, end_at, relative_path, codec,
		       width, height, size_bytes, protected, status, created_at
		FROM nvr_recording_segments
		WHERE id = ?
	`, strings.TrimSpace(id)).Scan(
		&record.ID, &record.CameraID, &record.StorageTargetID, &startAt, &endAt,
		&record.RelativePath, &record.Codec, &record.Width, &record.Height,
		&record.SizeBytes, &protected, &record.Status, &createdAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return NVRRecordingSegmentRecord{}, sql.ErrNoRows
	}
	if err != nil {
		return NVRRecordingSegmentRecord{}, fmt.Errorf("read NVR recording segment: %w", err)
	}
	record.Protected = protected == 1
	record.StartAt, err = time.Parse(time.RFC3339Nano, startAt)
	if err != nil {
		return NVRRecordingSegmentRecord{}, fmt.Errorf("parse NVR segment start_at: %w", err)
	}
	record.EndAt, err = time.Parse(time.RFC3339Nano, endAt)
	if err != nil {
		return NVRRecordingSegmentRecord{}, fmt.Errorf("parse NVR segment end_at: %w", err)
	}
	record.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return NVRRecordingSegmentRecord{}, fmt.Errorf("parse NVR segment created_at: %w", err)
	}
	return record, nil
}

func (s *Store) OldestNVRRetentionSegments(
	ctx context.Context,
	storageTargetID string,
	limit int,
) ([]NVRRecordingSegmentRecord, error) {
	if limit <= 0 {
		return []NVRRecordingSegmentRecord{}, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id
		FROM nvr_recording_segments
		WHERE storage_target_id = ?
		  AND protected = 0
		  AND status = 'complete'
		ORDER BY start_at ASC, id ASC
		LIMIT ?
	`, strings.TrimSpace(storageTargetID), limit)
	if err != nil {
		return nil, fmt.Errorf("list NVR retention segments: %w", err)
	}
	defer rows.Close()
	ids := make([]string, 0, limit)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan NVR retention segment: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate NVR retention segments: %w", err)
	}
	result := make([]NVRRecordingSegmentRecord, 0, len(ids))
	for _, id := range ids {
		record, err := s.NVRRecordingSegment(ctx, id)
		if err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, nil
}

func (s *Store) DeleteNVRRecordingSegment(ctx context.Context, id string) error {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM nvr_recording_segments WHERE id = ?", strings.TrimSpace(id)); err != nil {
		return fmt.Errorf("delete NVR recording segment: %w", err)
	}
	return nil
}

func (s *Store) NVRArchiveBytes(ctx context.Context, storageTargetID string) (int64, error) {
	var total sql.NullInt64
	if err := s.db.QueryRowContext(ctx, `
		SELECT SUM(size_bytes)
		FROM nvr_recording_segments
		WHERE storage_target_id = ? AND status = 'complete'
	`, strings.TrimSpace(storageTargetID)).Scan(&total); err != nil {
		return 0, fmt.Errorf("sum NVR archive bytes: %w", err)
	}
	if !total.Valid {
		return 0, nil
	}
	return total.Int64, nil
}
