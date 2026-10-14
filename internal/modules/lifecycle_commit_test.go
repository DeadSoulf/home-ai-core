package modules

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DeadSoulf/home-ai-core/internal/state"
)

// Simulate cancellation arriving after a privileged helper confirms an external
// effect, but before the registry has committed that effect to SQLite.
func TestControlCommitsConfirmedEffectAfterCancellation(t *testing.T) {
	tests := []struct {
		name, previous, action, helperAction, expected string
	}{
		{"start", "disabled", "enable", "docker.module.start", "enabled"},
		{"stop", "enabled", "disable", "docker.module.stop", "disabled"},
		{"restart", "enabled", "restart", "docker.module.restart", "enabled"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db, err := state.Open(context.Background(), t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			persistManifest(t, db, validManifest("demo", "1.0.0"), tc.previous)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			registry := NewRegistry(db)
			calls := 0
			registry.helper = func(helperCtx context.Context, operation, id, image string) (helperModuleResult, error) {
				calls++
				if operation != tc.helperAction || id != "demo" || image != "" {
					t.Fatalf("unexpected helper invocation %q %q %q", operation, id, image)
				}
				if helperCtx.Err() != nil {
					t.Fatal("helper called with already canceled context")
				}
				cancel() // happens only AFTER the side effect has completed
				return helperModuleResult{State: "running"}, nil
			}
			got, err := registry.Control(ctx, "demo", tc.action)
			if err != nil {
				t.Fatalf("Control() after helper success: %v", err)
			}
			if calls != 1 {
				t.Fatalf("helper called %d times, want 1", calls)
			}
			if got.Status != tc.expected || got.Error != "" {
				t.Fatalf("returned status = %q, error = %q; want %q and empty error", got.Status, got.Error, tc.expected)
			}
			persisted, err := db.Module(context.Background(), "demo")
			if err != nil {
				t.Fatal(err)
			}
			if persisted.Status != tc.expected || persisted.Error != "" {
				t.Fatalf("persisted status = %q, error = %q; want %q and empty error", persisted.Status, persisted.Error, tc.expected)
			}
		})
	}
}

func TestRemoveCommitsConfirmedEffectAfterCancellation(t *testing.T) {
	db, err := state.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	persistManifest(t, db, validManifest("demo", "1.0.0"), "enabled")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	registry := NewRegistry(db)
	calls := 0
	registry.helper = func(helperCtx context.Context, operation, id, image string) (helperModuleResult, error) {
		calls++
		if operation != "docker.module.remove" || id != "demo" || image != "" || helperCtx.Err() != nil {
			t.Fatalf("unexpected helper invocation %q %q %q", operation, id, image)
		}
		cancel() // container has already been removed
		return helperModuleResult{State: "removed"}, nil
	}
	if err := registry.Remove(ctx, "demo"); err != nil {
		t.Fatalf("Remove() after helper success: %v", err)
	}
	if calls != 1 {
		t.Fatalf("helper called %d times, want 1", calls)
	}
	_, err = db.Module(context.Background(), "demo")
	if !errors.Is(err, state.ErrModuleNotFound) {
		t.Fatalf("ghost module registration remains after removal: %v", err)
	}
}

func TestControlReportsPersistenceFailureAfterSuccessfulHelper(t *testing.T) {
	db, err := state.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	persistManifest(t, db, validManifest("demo", "1.0.0"), "disabled")

	failure := errors.New("simulated SQLite write failure")
	store := &brokenModuleStore{Store: db, statusErr: failure}
	registry := NewRegistry(store)
	calls := 0
	registry.helper = func(context.Context, string, string, string) (helperModuleResult, error) {
		calls++
		return helperModuleResult{State: "running"}, nil
	}

	_, err = registry.Control(context.Background(), "demo", "enable")
	if !errors.Is(err, failure) || !strings.Contains(err.Error(), "completed in Docker") {
		t.Fatalf("expected explicit, wrapped post-side-effect failure; got %v", err)
	}
	if calls != 1 {
		t.Fatalf("helper invoked %d times", calls)
	}
	item, err := db.Module(context.Background(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if item.Status != "disabled" {
		t.Fatalf("unexpected failed status mutation: %q", item.Status)
	}
}

type failingDeleteModuleStore struct {
	*state.Store
	deleteErr error
}

func (s *failingDeleteModuleStore) DeleteModule(ctx context.Context, id string) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	return s.Store.DeleteModule(ctx, id)
}

func TestRemoveReportsPersistenceFailureAndCanReconcile(t *testing.T) {
	db, err := state.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	persistManifest(t, db, validManifest("demo", "1.0.0"), "enabled")

	failure := errors.New("simulated SQLite delete failure")
	store := &failingDeleteModuleStore{Store: db, deleteErr: failure}
	registry := NewRegistry(store)
	calls := 0
	registry.helper = func(context.Context, string, string, string) (helperModuleResult, error) {
		calls++
		// The helper's remove is idempotent when the managed container is absent.
		return helperModuleResult{State: "removed"}, nil
	}
	err = registry.Remove(context.Background(), "demo")
	if !errors.Is(err, failure) || !strings.Contains(err.Error(), "container removed") {
		t.Fatalf("missing post-side-effect failure report: %v", err)
	}
	if _, err := db.Module(context.Background(), "demo"); err != nil {
		t.Fatalf("failed registry deletion unexpectedly lost the intent: %v", err)
	}

	store.deleteErr = nil
	if err := registry.Remove(context.Background(), "demo"); err != nil {
		t.Fatalf("cannot reconcile retained registration after helper removal: %v", err)
	}
	if calls != 2 {
		t.Fatalf("helper called %d times; expected one operation and one explicit retry", calls)
	}
	if _, err := db.Module(context.Background(), "demo"); !errors.Is(err, state.ErrModuleNotFound) {
		t.Fatalf("registry still contains removed module: %v", err)
	}
}

func TestCancelledHelperFailureDoesNotClaimCompletedControl(t *testing.T) {
	db, err := state.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	persistManifest(t, db, validManifest("demo", "1.0.0"), "enabled")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	failure := errors.New("helper operation not confirmed")
	registry := NewRegistry(db)
	registry.helper = func(context.Context, string, string, string) (helperModuleResult, error) {
		cancel()
		return helperModuleResult{}, failure
	}
	if _, err := registry.Control(ctx, "demo", "disable"); !errors.Is(err, failure) {
		t.Fatalf("lost helper failure: %v", err)
	}
	item, err := db.Module(context.Background(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if item.Status != "enabled" || !strings.Contains(item.Error, failure.Error()) {
		t.Fatalf("unconfirmed helper call was incorrectly committed: status=%q error=%q", item.Status, item.Error)
	}
}
