package state

import (
	"context"
	"encoding/base64"
	"testing"
	"time"
)

func TestModuleStoreConfigurationPersists(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	if err := store.AddModuleSigningKey(ctx, ModuleSigningKeyRecord{
		KeyID: "official-2026", Algorithm: "ed25519", PublicKeyB64: key,
	}); err != nil {
		t.Fatalf("AddModuleSigningKey() error = %v", err)
	}
	if err := store.UpsertModuleRepository(ctx, ModuleRepositoryRecord{
		ID: "official", IndexURL: "https://modules.example.invalid/index.json",
		SignatureURL: "https://modules.example.invalid/index.json.sig",
		KeyID: "official-2026", Enabled: true,
	}); err != nil {
		t.Fatalf("UpsertModuleRepository() error = %v", err)
	}

	keys, err := store.ListModuleSigningKeys(ctx)
	if err != nil || len(keys) != 1 || keys[0].KeyID != "official-2026" {
		t.Fatalf("unexpected keys: %#v err=%v", keys, err)
	}
	repos, err := store.ListModuleRepositories(ctx)
	if err != nil || len(repos) != 1 || !repos[0].Enabled {
		t.Fatalf("unexpected repositories: %#v err=%v", repos, err)
	}

	refreshed := time.Now().UTC()
	if err := store.SaveModuleRepositorySnapshot(ctx, "official", `{"schema_version":1}`, `{"key_id":"official-2026"}`, "", refreshed); err != nil {
		t.Fatalf("SaveModuleRepositorySnapshot() error = %v", err)
	}
	repos, err = store.ListModuleRepositories(ctx)
	if err != nil || repos[0].LastIndexJSON == "" || repos[0].LastRefreshedAt == nil {
		t.Fatalf("snapshot was not persisted: %#v err=%v", repos, err)
	}
}
