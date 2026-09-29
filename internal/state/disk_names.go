package state

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func (s *Store) DiskNames(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT stable_id, display_name FROM disk_names")
	if err != nil {
		return nil, fmt.Errorf("list disk names: %w", err)
	}
	defer rows.Close()

	result := map[string]string{}
	for rows.Next() {
		var stableID, displayName string
		if err := rows.Scan(&stableID, &displayName); err != nil {
			return nil, fmt.Errorf("scan disk name: %w", err)
		}
		result[stableID] = displayName
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate disk names: %w", err)
	}
	return result, nil
}

func (s *Store) SetDiskName(ctx context.Context, stableID, displayName string) error {
	stableID = strings.TrimSpace(stableID)
	displayName = strings.TrimSpace(displayName)
	if stableID == "" {
		return fmt.Errorf("disk stable id is required")
	}
	if displayName == "" {
		if _, err := s.db.ExecContext(ctx, "DELETE FROM disk_names WHERE stable_id = ?", stableID); err != nil {
			return fmt.Errorf("delete disk name: %w", err)
		}
		return nil
	}
	if len([]rune(displayName)) > 64 {
		return fmt.Errorf("disk name is too long")
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO disk_names(stable_id, display_name, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(stable_id) DO UPDATE SET
			display_name = excluded.display_name,
			updated_at = excluded.updated_at
	`, stableID, displayName, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("save disk name: %w", err)
	}
	return nil
}
