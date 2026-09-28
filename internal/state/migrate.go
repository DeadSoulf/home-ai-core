package state

import (
	"context"
	"embed"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type migration struct {
	version int
	name    string
	sql     string
}

func (s *Store) ApplyMigrations(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TEXT NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("initialize migration registry: %w", err)
	}

	migrations, err := loadMigrations()
	if err != nil {
		return err
	}

	for _, item := range migrations {
		applied, err := s.migrationApplied(ctx, item.version)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		if err := s.applyMigration(ctx, item); err != nil {
			return err
		}
	}

	return nil
}

func loadMigrations() ([]migration, error) {
	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return nil, fmt.Errorf("read embedded migrations: %w", err)
	}

	items := make([]migration, 0, len(entries))
	seen := map[int]struct{}{}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		version, err := migrationVersion(entry.Name())
		if err != nil {
			return nil, err
		}
		if _, exists := seen[version]; exists {
			return nil, fmt.Errorf("duplicate migration version %d", version)
		}
		seen[version] = struct{}{}

		content, err := migrationFiles.ReadFile(path.Join("migrations", entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}

		items = append(items, migration{
			version: version,
			name:    entry.Name(),
			sql:     string(content),
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].version < items[j].version
	})

	return items, nil
}

func migrationVersion(name string) (int, error) {
	prefix, _, ok := strings.Cut(name, "_")
	if !ok {
		return 0, fmt.Errorf("migration %q must use <version>_<name>.sql", name)
	}

	version, err := strconv.Atoi(prefix)
	if err != nil || version <= 0 {
		return 0, fmt.Errorf("migration %q has invalid version", name)
	}
	return version, nil
}

func (s *Store) migrationApplied(ctx context.Context, version int) (bool, error) {
	var count int
	if err := s.db.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM schema_migrations WHERE version = ?",
		version,
	).Scan(&count); err != nil {
		return false, fmt.Errorf("check migration %d: %w", version, err)
	}
	return count == 1, nil
}

func (s *Store) applyMigration(ctx context.Context, item migration) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %d: %w", item.version, err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.ExecContext(ctx, item.sql); err != nil {
		return fmt.Errorf("apply migration %d (%s): %w", item.version, item.name, err)
	}

	if _, err := tx.ExecContext(
		ctx,
		"INSERT INTO schema_migrations(version, name, applied_at) VALUES (?, ?, ?)",
		item.version,
		item.name,
		time.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		return fmt.Errorf("record migration %d: %w", item.version, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %d: %w", item.version, err)
	}
	return nil
}
