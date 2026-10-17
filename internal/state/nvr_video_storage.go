package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// NVRVideoStorage is recorded only after the privileged helper confirms the
// new NVR container is healthy and its /data/recordings bind is active.
type NVRVideoStorage struct {
	DevicePath     string `json:"device"`
	FilesystemUUID string `json:"filesystem_uuid"`
	Mountpoint     string `json:"mountpoint"`
	ActiveAt       string `json:"active_at"`
}

func (s *Store) NVRVideoStorage(ctx context.Context) (NVRVideoStorage, bool, error) {
	var item NVRVideoStorage
	err := s.db.QueryRowContext(ctx,
		`SELECT device_path, filesystem_uuid, mountpoint, active_at FROM nvr_video_storage WHERE singleton=1`,
	).Scan(&item.DevicePath, &item.FilesystemUUID, &item.Mountpoint, &item.ActiveAt)
	if errors.Is(err, sql.ErrNoRows) {
		return NVRVideoStorage{}, false, nil
	}
	if err != nil {
		return NVRVideoStorage{}, false, fmt.Errorf("read NVR video storage: %w", err)
	}
	return item, true, nil
}

func (s *Store) SetNVRVideoStorage(ctx context.Context, item NVRVideoStorage) error {
	if strings.TrimSpace(item.DevicePath) == "" ||
		strings.TrimSpace(item.FilesystemUUID) == "" ||
		strings.TrimSpace(item.Mountpoint) == "" {
		return errors.New("NVR storage identity is incomplete")
	}
	if item.ActiveAt == "" {
		item.ActiveAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO nvr_video_storage
		(singleton, device_path, filesystem_uuid, mountpoint, active_at)
		VALUES (1, ?, ?, ?, ?)
		ON CONFLICT(singleton) DO UPDATE SET
			device_path=excluded.device_path,
			filesystem_uuid=excluded.filesystem_uuid,
			mountpoint=excluded.mountpoint,
			active_at=excluded.active_at`,
		item.DevicePath, item.FilesystemUUID, item.Mountpoint, item.ActiveAt)
	if err != nil {
		return fmt.Errorf("persist NVR video storage: %w", err)
	}
	return nil
}
