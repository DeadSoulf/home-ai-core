package modules

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/DeadSoulf/home-ai-core/internal/state"
)

func aiPatchFixture(version, digest string) Manifest {
	m := validManifest("ai.agent", version)
	m.Runtime.Health = HealthSpec{Port: 8080, Path: "/health"}
	m.Runtime.Docker.Image = "ghcr.io/deadsoulf/home-ai-core-ai@sha256:" + strings.Repeat(digest, 64)
	m.Host.Architectures = []string{"amd64", "arm64"}
	m.Lifecycle = []string{"install", "upgrade", "remove", "start", "stop", "restart"}
	return m
}

func TestAIPatchUpgradeRejectsOtherModulesAndDowngrades(t *testing.T) {
	current := aiPatchFixture("0.3.0", "a")
	next := aiPatchFixture("0.3.1", "b")
	if err := validateAIUpgrade(current, next, "enabled"); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name                string
		previous, candidate Manifest
		status              string
	}{
		{"same version", current, current, "enabled"},
		{"older version", next, current, "enabled"},
		{"different module", current, validManifest("nvr.module", "0.3.1"), "enabled"},
		{"legacy version", aiPatchFixture("0.2.0", "a"), next, "enabled"},
		{"disabled", current, next, "disabled"},
		{"incomplete", current, next, "registered"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if validateAIUpgrade(tc.previous, tc.candidate, tc.status) == nil {
				t.Fatal("unsafe module replacement accepted")
			}
		})
	}
}

func TestRegistryAIPatchUpgradeCommitsOnlyAfterHelper(t *testing.T) {
	ctx := context.Background()
	store, err := state.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	old := aiPatchFixture("0.3.0", "a")
	next := aiPatchFixture("0.3.1", "b")
	persistManifest(t, store, old, "enabled")
	registry := NewRegistry(store, "0.1.195-dev")
	var operations []string
	registry.upgradeHelper = func(_ context.Context, op, previous, nextImage, raw string, report func(string, string)) (helperModuleResult, error) {
		operations = append(operations, op)
		switch op {
		case "docker.module.upgrade":
			if previous != old.Runtime.Docker.Image || nextImage != next.Runtime.Docker.Image || !strings.Contains(raw, "0.3.0") {
				t.Fatal("wrong immutable source manifest")
			}
			// Still the old version when Docker declares the new candidate healthy.
			record, err := store.Module(ctx, "ai.agent")
			if err != nil || record.Version != "0.3.0" {
				t.Fatalf("registry advanced before health: %+v %v", record, err)
			}
			return helperModuleResult{State: "running"}, nil
		case "docker.module.upgrade.finalize":
			record, err := store.Module(ctx, "ai.agent")
			if err != nil || record.Version != "0.3.1" {
				t.Fatalf("finalized before registry commit: %+v %v", record, err)
			}
			return helperModuleResult{State: "committed"}, nil
		}
		t.Fatalf("unexpected operation %s", op)
		return helperModuleResult{}, nil
	}
	result, err := registry.UpgradeManifest(ctx, next, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Manifest.Version != "0.3.1" || result.Status != "enabled" {
		t.Fatalf("wrong result %+v", result)
	}
	if strings.Join(operations, ",") != "docker.module.upgrade,docker.module.upgrade.finalize" {
		t.Fatalf("wrong operation sequence %v", operations)
	}
}

func TestRegistryFailedAIUpdatePreservesOldRegistration(t *testing.T) {
	ctx := context.Background()
	store, err := state.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	old := aiPatchFixture("0.3.0", "a")
	next := aiPatchFixture("0.3.1", "b")
	persistManifest(t, store, old, "enabled")
	r := NewRegistry(store)
	r.upgradeHelper = func(_ context.Context, op, previous, nextImage, raw string, report func(string, string)) (helperModuleResult, error) {
		return helperModuleResult{}, errors.New("candidate health check failed and old container restored")
	}
	if _, err := r.UpgradeManifest(ctx, next, nil); err == nil {
		t.Fatal("unhealthy candidate was accepted")
	}
	item, err := r.Get(ctx, "ai.agent")
	if err != nil || item.Status != "enabled" || item.Manifest.Version != "0.3.0" {
		t.Fatalf("old module registration was changed: %+v %v", item, err)
	}
}

func TestRegistryStartupRecoversOldAIAfterInterruptedUpgrade(t *testing.T) {
	ctx := context.Background()
	store, err := state.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	old := aiPatchFixture("0.3.0", "a")
	newer := aiPatchFixture("0.3.1", "b")
	persistManifest(t, store, newer, "enabled")
	r := NewRegistry(store)
	raw, _ := json.Marshal(old)
	r.upgradeHelper = func(_ context.Context, op, prev, newImage, rawPrev string, _ func(string, string)) (helperModuleResult, error) {
		if op != "docker.module.upgrade.reconcile" || newImage != newer.Runtime.Docker.Image {
			t.Fatalf("wrong startup reconcile: %s %s", op, newImage)
		}
		return helperModuleResult{State: "rolled_back", PreviousManifest: string(raw)}, nil
	}
	if err := r.RecoverPendingAIUpgrade(ctx); err != nil {
		t.Fatal(err)
	}
	record, err := store.Module(ctx, "ai.agent")
	if err != nil || record.Version != "0.3.0" || record.Status != "enabled" {
		t.Fatalf("journal rollback not persisted: %+v %v", record, err)
	}
}
