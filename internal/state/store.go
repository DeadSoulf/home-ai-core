package state

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

const databaseFile = "core.db"

type Store struct {
	db *sql.DB
}

func Open(ctx context.Context, stateDir string) (*Store, error) {
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return nil, fmt.Errorf("create state directory: %w", err)
	}

	databasePath := filepath.Join(stateDir, databaseFile)
	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	// Keep the initial control-plane database deliberately simple and
	// deterministic. A single writer is enough for the first Core phases and
	// makes connection-scoped SQLite pragmas reliable.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	store := &Store{db: db}
	if err := store.configure(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.ApplyMigrations(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

func (s *Store) configure(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	statements := []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA journal_mode = WAL",
	}

	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("configure sqlite (%s): %w", statement, err)
		}
	}

	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping sqlite database: %w", err)
	}
	return nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *Store) SchemaVersion(ctx context.Context) (int, error) {
	var version sql.NullInt64
	err := s.db.QueryRowContext(ctx, "SELECT MAX(version) FROM schema_migrations").Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}
	if !version.Valid {
		return 0, nil
	}
	return int(version.Int64), nil
}
