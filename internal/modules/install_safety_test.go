package modules

import (
	"context"
	"errors"
	"strings"
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

func TestInstallManifestHelperFailureCreatesNoGhostRegistration(t *testing.T) {
	ctx := context.Background()
	store, err := state.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	registry := NewRegistry(store, "0.1.172-dev")
	failure := errors.New("Docker start failed")
	registry.helper = func(context.Context, string, string, string) (helperModuleResult, error) {
		return helperModuleResult{}, failure
	}
	_, err = registry.InstallManifest(ctx, validManifest("demo", "1.0.0"))
	if !errors.Is(err, failure) {
		t.Fatalf("install error = %v, want runtime error", err)
	}
	if _, err := store.Module(ctx, "demo"); !errors.Is(err, state.ErrModuleNotFound) {
		t.Fatalf("failed install persisted a ghost module: %v", err)
	}
}

func TestInstallManifestStateFailureCompensatesRuntime(t *testing.T) {
	for _, failurePoint := range []string{"upsert", "enabled"} {
		t.Run(failurePoint, func(t *testing.T) {
			ctx := context.Background()
			underlying, err := state.Open(ctx, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer underlying.Close()
			store := &brokenModuleStore{Store: underlying}
			failure := errors.New("simulated SQLite failure")
			if failurePoint == "upsert" {
				store.upsertErr = failure
			} else {
				store.statusErr = failure
			}
			registry := NewRegistry(store, "0.1.172-dev")
			operations := []string{}
			registry.helper = func(_ context.Context, operation, _, _ string) (helperModuleResult, error) {
				operations = append(operations, operation)
				return helperModuleResult{State: "running"}, nil
			}
			_, err = registry.InstallManifest(ctx, validManifest("demo", "1.0.0"))
			if !errors.Is(err, failure) {
				t.Fatalf("install error = %v, want SQLite failure", err)
			}
			if strings.Join(operations, ",") != "docker.module.install,docker.module.remove" {
				t.Fatalf("expected compensating removal; operations = %#v", operations)
			}
			if _, err := underlying.Module(ctx, "demo"); !errors.Is(err, state.ErrModuleNotFound) {
				t.Fatalf("failed registration was not cleaned up: %v", err)
			}
		})
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
