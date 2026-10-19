package modules

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DeadSoulf/home-ai-core/internal/state"
)

func upgradeFixture(t *testing.T) (*state.Store, *Registry, Manifest) {
	t.Helper()
	ctx := context.Background()
	db, err := state.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	old := validManifest("ai.agent", "0.3.2")
	old.Lifecycle = append(old.Lifecycle, "upgrade")
	persistManifest(t, db, old, "enabled")
	return db, NewRegistry(db, "0.1.200"), old
}

func TestUpgradeAIRequiresNewerImmutableImage(t *testing.T) {
	db, registry, old := upgradeFixture(t)
	defer db.Close()
	calls := 0
	registry.upgradeHelper = func(context.Context, string, string, string, string, func(string, string)) (helperModuleResult, error) {
		calls++
		return helperModuleResult{State: "running"}, nil
	}
	if _, err := registry.UpgradeManifest(context.Background(), old); err == nil {
		t.Fatal("identical manifest accepted as an upgrade")
	}
	same := old
	same.Runtime.Docker.Image = "ghcr.io/home-ai/test@sha256:" + strings.Repeat("b", 64)
	if _, err := registry.UpgradeManifest(context.Background(), same); err == nil {
		t.Fatal("same version with a changed image accepted")
	}
	if calls != 0 {
		t.Fatalf("helper invoked %d times", calls)
	}
	item, err := registry.Get(context.Background(), old.ID)
	if err != nil || item.Manifest.Version != old.Version {
		t.Fatalf("running module changed on rejected upgrade: %#v %v", item, err)
	}
}

func TestUpgradeAIPersistsOnlyAfterHealthyContainer(t *testing.T) {
	db, registry, old := upgradeFixture(t)
	defer db.Close()
	next := old
	next.Version = "0.3.3"
	next.Runtime.Docker.Image = "ghcr.io/home-ai/test@sha256:" + strings.Repeat("b", 64)
	steps := []string{}
	registry.upgradeHelper = func(ctx context.Context, op, id, image, previous string, _ func(string, string)) (helperModuleResult, error) {
		if id != old.ID || image != next.Runtime.Docker.Image || previous != old.Runtime.Docker.Image {
			t.Fatal("helper used incorrect immutable images")
		}
		steps = append(steps, op)
		return helperModuleResult{State: "running"}, nil
	}
	updated, err := registry.UpgradeManifest(context.Background(), next)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Manifest.Version != "0.3.3" || updated.Status != "enabled" {
		t.Fatalf("unexpected updated module: %#v", updated)
	}
	if len(steps) != 2 || steps[0] != "docker.module.upgrade" || steps[1] != "docker.module.upgrade.commit" {
		t.Fatalf("unsafe helper sequence: %#v", steps)
	}
}

func TestUpgradeAIDoesNotPersistFailedCandidate(t *testing.T) {
	db, registry, old := upgradeFixture(t)
	defer db.Close()
	next := old
	next.Version = "0.3.3"
	next.Runtime.Docker.Image = "ghcr.io/home-ai/test@sha256:" + strings.Repeat("b", 64)
	failure := errors.New("new image unhealthy")
	registry.upgradeHelper = func(ctx context.Context, op, id, image, previous string, _ func(string, string)) (helperModuleResult, error) {
		return helperModuleResult{}, failure
	}
	if _, err := registry.UpgradeManifest(context.Background(), next); !errors.Is(err, failure) {
		t.Fatalf("expected candidate failure, got %v", err)
	}
	item, err := registry.Get(context.Background(), old.ID)
	if err != nil || item.Manifest.Version != old.Version {
		t.Fatalf("old version was not preserved: %#v %v", item, err)
	}
}

type failingUpgradeStore struct {
	*state.Store
	fail bool
}

func (s *failingUpgradeStore) UpsertModule(ctx context.Context, record state.ModuleRecord) error {
	if s.fail {
		return errors.New("simulated SQLite write failure")
	}
	return s.Store.UpsertModule(ctx, record)
}

func TestUpgradeAIRollsBackWhenRegistryWriteFails(t *testing.T) {
	db, _, old := upgradeFixture(t)
	defer db.Close()
	wrapper := &failingUpgradeStore{Store: db, fail: true}
	registry := NewRegistry(wrapper, "0.1.200")
	next := old
	next.Version = "0.3.3"
	next.Runtime.Docker.Image = "ghcr.io/home-ai/test@sha256:" + strings.Repeat("b", 64)
	steps := []string{}
	registry.upgradeHelper = func(ctx context.Context, op, id, image, previous string, _ func(string, string)) (helperModuleResult, error) {
		steps = append(steps, op)
		return helperModuleResult{State: "running"}, nil
	}
	if _, err := registry.UpgradeManifest(context.Background(), next); err == nil {
		t.Fatal("registry failure was swallowed")
	}
	if len(steps) != 2 || steps[0] != "docker.module.upgrade" || steps[1] != "docker.module.upgrade.rollback" {
		t.Fatalf("rollback not invoked: %#v", steps)
	}
	item, err := registry.Get(context.Background(), old.ID)
	if err != nil || item.Manifest.Version != old.Version {
		t.Fatalf("old registry manifest was not preserved: %#v %v", item, err)
	}
}

func TestUpgradeSupportsNVRAndReferenceModules(t *testing.T) {
	for _, moduleID := range []string{"nvr.module", "reference.module"} {
		t.Run(moduleID, func(t *testing.T) {
			ctx := context.Background()
			db, err := state.Open(ctx, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			old := validManifest(moduleID, "1.1.4")
			old.Lifecycle = append(old.Lifecycle, "upgrade")
			persistManifest(t, db, old, "enabled")
			registry := NewRegistry(db, "0.1.203")
			newer := old
			newer.Version = "1.1.5"
			newer.Runtime.Docker.Image = "ghcr.io/home-ai/test@sha256:" + strings.Repeat("b", 64)
			steps := []string{}
			registry.upgradeHelper = func(_ context.Context, op, id, image, previous string, _ func(string, string)) (helperModuleResult, error) {
				if id != moduleID || image != newer.Runtime.Docker.Image || previous != old.Runtime.Docker.Image {
					t.Fatalf("unsafe upgrade request: %s %s %s %s", op, id, image, previous)
				}
				steps = append(steps, op)
				return helperModuleResult{State: "running"}, nil
			}
			updated, err := registry.UpgradeManifest(ctx, newer)
			if err != nil {
				t.Fatal(err)
			}
			if updated.Manifest.Version != "1.1.5" || updated.Status != "enabled" {
				t.Fatalf("unexpected result: %#v", updated)
			}
			if len(steps) != 2 || steps[0] != "docker.module.upgrade" || steps[1] != "docker.module.upgrade.commit" {
				t.Fatalf("unsafe upgrade sequence: %v", steps)
			}
		})
	}
}

func TestUpgradeRejectsWithoutLifecycleOptIn(t *testing.T) {
	ctx := context.Background()
	for _, missingIn := range []string{"installed", "candidate"} {
		db, err := state.Open(ctx, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		old := validManifest("nvr.module", "1.1.4")
		if missingIn == "candidate" {
			old.Lifecycle = append(old.Lifecycle, "upgrade")
		}
		persistManifest(t, db, old, "enabled")
		next := old
		next.Version = "1.1.5"
		next.Runtime.Docker.Image = "ghcr.io/home-ai/test@sha256:" + strings.Repeat("b", 64)
		if missingIn == "installed" {
			next.Lifecycle = append(next.Lifecycle, "upgrade")
		} else {
			next.Lifecycle = []string{"install", "remove"}
		}
		registry := NewRegistry(db, "0.1.203")
		calls := 0
		registry.upgradeHelper = func(context.Context, string, string, string, string, func(string, string)) (helperModuleResult, error) {
			calls++
			return helperModuleResult{}, nil
		}
		if _, err := registry.UpgradeManifest(ctx, next); err == nil || !strings.Contains(err.Error(), "upgrade capability") {
			t.Fatalf("lifecycle opt-in ignored: %v", err)
		}
		if calls != 0 {
			t.Fatal("unsafe Docker helper invoked")
		}
		db.Close()
	}
}
