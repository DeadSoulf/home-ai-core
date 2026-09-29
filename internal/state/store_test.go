package state

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenAppliesMigrationsAndPersistsNode(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	store, err := Open(ctx, dir)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	version, err := store.SchemaVersion(ctx)
	if err != nil {
		t.Fatalf("SchemaVersion() error = %v", err)
	}
	if version != 7 {
		t.Fatalf("schema version = %d, want 7", version)
	}

	const nodeID = "00000000-0000-4000-8000-000000000001"
	if err := store.EnsureNode(ctx, nodeID, "home-ai-test"); err != nil {
		t.Fatalf("EnsureNode() error = %v", err)
	}

	first, err := store.Node(ctx, nodeID)
	if err != nil {
		t.Fatalf("Node() error = %v", err)
	}
	if first.Hostname != "home-ai-test" {
		t.Fatalf("hostname = %q", first.Hostname)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened, err := Open(ctx, dir)
	if err != nil {
		t.Fatalf("reopen error = %v", err)
	}
	defer reopened.Close()

	second, err := reopened.Node(ctx, nodeID)
	if err != nil {
		t.Fatalf("Node() after reopen error = %v", err)
	}

	if !second.CreatedAt.Equal(first.CreatedAt) {
		t.Fatalf("created_at changed: %s != %s", second.CreatedAt, first.CreatedAt)
	}

	// Ensure a normal startup refresh does not recreate the record.
	time.Sleep(time.Millisecond)
	if err := reopened.EnsureNode(ctx, nodeID, "renamed-node"); err != nil {
		t.Fatalf("EnsureNode() refresh error = %v", err)
	}
	updated, err := reopened.Node(ctx, nodeID)
	if err != nil {
		t.Fatalf("updated Node() error = %v", err)
	}
	if updated.Hostname != "renamed-node" {
		t.Fatalf("updated hostname = %q", updated.Hostname)
	}
	if !updated.LastSeenAt.After(second.LastSeenAt) {
		t.Fatalf("last_seen_at was not refreshed")
	}

	if _, err := filepath.Abs(filepath.Join(dir, databaseFile)); err != nil {
		t.Fatalf("database path invalid: %v", err)
	}
}

func TestMigrationsAreIdempotent(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	store, err := Open(ctx, dir)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	if err := store.ApplyMigrations(ctx); err != nil {
		t.Fatalf("second ApplyMigrations() error = %v", err)
	}

	var count int
	if err := store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if count != 7 {
		t.Fatalf("migration rows = %d, want 7", count)
	}
}

func TestModuleRegistrationPreservesStatus(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	if err := store.UpsertModule(ctx, ModuleRecord{
		ID:           "storage",
		Version:      "1.0.0",
		Status:       "enabled",
		ManifestJSON: `{"id":"storage","version":"1.0.0"}`,
	}); err != nil {
		t.Fatalf("first UpsertModule() error = %v", err)
	}
	if err := store.UpsertModule(ctx, ModuleRecord{
		ID:           "storage",
		Version:      "1.1.0",
		Status:       "registered",
		ManifestJSON: `{"id":"storage","version":"1.1.0"}`,
	}); err != nil {
		t.Fatalf("second UpsertModule() error = %v", err)
	}

	record, err := store.Module(ctx, "storage")
	if err != nil {
		t.Fatalf("Module() error = %v", err)
	}
	if record.Status != "enabled" {
		t.Fatalf("status = %q, want enabled", record.Status)
	}
	if record.Version != "1.1.0" {
		t.Fatalf("version = %q, want 1.1.0", record.Version)
	}
}
