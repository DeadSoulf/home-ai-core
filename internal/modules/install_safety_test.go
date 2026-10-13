package modules

import (
	"context"
	"errors"
	"testing"

	"github.com/DeadSoulf/home-ai-core/internal/state"
)

type brokenModuleStore struct {
	*state.Store
	upsertErr error
	statusErr error
}

func (s *brokenModuleStore) UpsertModule(ctx context.Context, record state.ModuleRecord) error {
	if s.upsertErr != nil {
		return s.upsertErr
	}
	return s.Store.UpsertModule(ctx, record)
}

func (s *brokenModuleStore) SetModuleStatus(ctx context.Context, id, status, message string) error {
	if status == "enabled" && s.statusErr != nil {
		return s.statusErr
	}
	return s.Store.SetModuleStatus(ctx, id, status, message)
}

func TestInstallManifestRejectsReplacementWithoutMutatingRegistration(t *testing.T) {
	ctx := context.Background()
	store, err := state.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	existing := validManifest("demo", "1.0.0")
	persistManifest(t, store, existing, "enabled")
	registry := NewRegistry(store, "0.1.172-dev")
	called := false
	registry.helper = func(context.Context, string, string, string) (helperModuleResult, error) {
		called = true
		return helperModuleResult{}, nil
	}
	newer := validManifest("demo", "1.1.0")
	_, err = registry.InstallManifest(ctx, newer)
	if !errors.Is(err, ErrModuleAlreadyInstalled) {
		t.Fatalf("upgrade error = %v, want safety rejection", err)
	}
	if called {
		t.Fatal("helper was invoked while replacing an installed module")
	}
	current, err := registry.Get(ctx, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if current.Manifest.Version != existing.Version || current.Status != "enabled" {
		t.Fatalf("last known-good registration changed: %#v", current)
	}
}

func TestInstallManifestFailureRetainsRecoverableIntent(t *testing.T) {
	ctx := context.Background()
	store, err := state.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	registry := NewRegistry(store, "0.1.172-dev")
	manifest := validManifest("demo", "1.0.0")
	failure := errors.New("lost helper reply")
	registry.helper = func(context.Context, string, string, string) (helperModuleResult, error) {
		return helperModuleResult{}, failure
	}
	if _, err := registry.InstallManifest(ctx, manifest); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	pending, err := registry.Get(ctx, "demo")
	if err != nil || pending.Status != "registered" || pending.Manifest.Version != "1.0.0" {
		t.Fatalf("missing durable intent %#v %v", pending, err)
	}
	// A new Core instance resumes exactly that manifest after a process crash.
	recovered := NewRegistry(store, "0.1.172-dev")
	recovered.helper = func(context.Context, string, string, string) (helperModuleResult, error) {
		return helperModuleResult{State: "running"}, nil
	}
	ready, err := recovered.InstallManifest(ctx, manifest)
	if err != nil || ready.Status != "enabled" {
		t.Fatalf("resume failed %#v %v", ready, err)
	}
	if _, err := recovered.InstallManifest(ctx, manifest); err != nil {
		t.Fatalf("completed install replay is not idempotent: %v", err)
	}
}

func TestInstallManifestStateFailureRetainsIntent(t *testing.T) {
	ctx := context.Background()
	db, err := state.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	failure := errors.New("simulated SQLite failure")
	store := &brokenModuleStore{Store: db, upsertErr: failure}
	registry := NewRegistry(store, "0.1.172-dev")
	calls := 0
	registry.helper = func(context.Context, string, string, string) (helperModuleResult, error) {
		calls++
		return helperModuleResult{State: "running"}, nil
	}
	manifest := validManifest("demo", "1.0.0")
	if _, err := registry.InstallManifest(ctx, manifest); !errors.Is(err, failure) || calls != 0 {
		t.Fatalf("side effects preceded durable intent: %v calls=%d", err, calls)
	}
	store.upsertErr = nil
	store.statusErr = failure
	if _, err := registry.InstallManifest(ctx, manifest); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	pending, err := db.Module(ctx, "demo")
	if err != nil || pending.Status != "registered" {
		t.Fatal("lost pending record")
	}
	store.statusErr = nil
	if _, err := registry.InstallManifest(ctx, manifest); err != nil {
		t.Fatal(err)
	}
}

func TestPendingInstallAdvertisesNoCapabilities(t *testing.T) {
	ctx := context.Background()
	db, err := state.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	manifest := validManifest("demo", "1.0.0")
	manifest.Capabilities.Provides = []string{"demo.ready"}
	persistManifest(t, db, manifest, "registered")
	values, err := NewRegistry(db).Capabilities(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if contains(values, "demo.ready") {
		t.Fatal("unfinished install advertised a running capability")
	}
}

func TestInstallManifestSuccessPersistsOnlyEnabledState(t *testing.T) {
	ctx := context.Background()
	store, err := state.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	registry := NewRegistry(store, "0.1.172-dev")
	registry.helper = func(_ context.Context, operation, id, image string) (helperModuleResult, error) {
		if operation != "docker.module.install" || id != "demo" || image == "" {
			t.Fatalf("unexpected helper call %q %q %q", operation, id, image)
		}
		return helperModuleResult{State: "running"}, nil
	}
	item, err := registry.InstallManifest(ctx, validManifest("demo", "1.0.0"))
	if err != nil {
		t.Fatal(err)
	}
	if item.Status != "enabled" {
		t.Fatalf("installed status = %q, want enabled", item.Status)
	}
}
