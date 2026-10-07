package modules

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/DeadSoulf/home-ai-core/internal/state"
)

func validManifest(id, ver string) Manifest {
	return Manifest{
		SchemaVersion: ManifestSchemaVersion,
		ID:            id,
		Name:          "Test Module",
		Version:       ver,
		Core:          ">=0.1.0 <1.0.0",
		Runtime: RuntimeSpec{
			Type: "docker",
			Docker: DockerSpec{
				Image: "ghcr.io/home-ai/test@sha256:" + strings.Repeat("a", 64),
			},
		},
		Capabilities: Capabilities{
			Requires: []string{"host.linux", "host.docker"},
		},
		UI: UIContract{
			Navigation: []NavigationItem{
				{ID: "overview", Title: "Overview", Route: "/modules/" + id},
			},
		},
		Lifecycle: []string{"install", "remove"},
	}
}

func persistManifest(t *testing.T, store *state.Store, manifest Manifest, status string) {
	t.Helper()
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertModule(context.Background(), state.ModuleRecord{
		ID:           manifest.ID,
		Version:      manifest.Version,
		Status:       status,
		ManifestJSON: string(raw),
	}); err != nil {
		t.Fatal(err)
	}
	if status != "registered" {
		if err := store.SetModuleStatus(context.Background(), manifest.ID, status, ""); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDecodeManifestRejectsUnknownField(t *testing.T) {
	raw := `{
		"schema_version":2,
		"id":"test",
		"name":"Test",
		"version":"0.1.0",
		"core":">=0.1.0 <1.0.0",
		"runtime":{"type":"docker","docker":{"image":"ghcr.io/home-ai/test@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},
		"capabilities":{"requires":["host.docker"]},
		"lifecycle":["install"],
		"unknown":true
	}`
	if _, err := DecodeManifest(strings.NewReader(raw)); err == nil {
		t.Fatal("unknown manifest field was accepted")
	}
}

func TestValidateManifestRejectsRouteEscape(t *testing.T) {
	m := validManifest("storage", "1.0.0")
	m.UI.Navigation[0].Route = "/system"
	if err := ValidateManifest(m); err == nil {
		t.Fatal("navigation route outside module namespace was accepted")
	}
}

func TestPlanInstallDependencyFirst(t *testing.T) {
	base := validManifest("base", "1.2.0")
	base.Capabilities.Provides = []string{"runtime.base"}
	base.Capabilities.Requires = []string{"host.linux", "host.docker"}

	app := validManifest("feature", "2.0.0")
	app.Dependencies = []Dependency{{ID: "base", Version: ">=1.0.0 <2.0.0"}}
	app.Capabilities.Requires = []string{"host.docker", "runtime.base"}

	plan, err := PlanInstall(PlanInput{
		CoreVersion:  "0.1.0",
		Target:       "feature",
		Available:    map[string]Manifest{"base": base, "feature": app},
		Capabilities: []string{"host.linux", "host.docker"},
		Architecture: "amd64",
	})
	if err != nil {
		t.Fatalf("PlanInstall() error = %v", err)
	}
	if len(plan.Order) != 2 || plan.Order[0].ID != "base" || plan.Order[1].ID != "feature" {
		t.Fatalf("unexpected plan: %#v", plan.Order)
	}
}

func TestPlanInstallRejectsCycle(t *testing.T) {
	a := validManifest("a", "1.0.0")
	b := validManifest("b", "1.0.0")
	a.Dependencies = []Dependency{{ID: "b", Version: ">=1.0.0"}}
	b.Dependencies = []Dependency{{ID: "a", Version: ">=1.0.0"}}

	_, err := PlanInstall(PlanInput{
		CoreVersion:  "0.1.0",
		Target:       "a",
		Available:    map[string]Manifest{"a": a, "b": b},
		Capabilities: []string{"host.linux", "host.docker"},
		Architecture: "amd64",
	})
	if err == nil {
		t.Fatal("dependency cycle was accepted")
	}
}

func TestRegistryPersistsManifest(t *testing.T) {
	ctx := context.Background()
	store, err := state.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("state.Open() error = %v", err)
	}
	defer store.Close()

	registry := NewRegistry(store)
	manifest := validManifest("storage", "1.0.0")
	persistManifest(t, store, manifest, "registered")

	got, err := registry.Get(ctx, "storage")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Manifest.Version != "1.0.0" || got.Status != "registered" {
		t.Fatalf("unexpected registered module: %#v", got)
	}
}

func TestRegistrySetStatusPersistsDisabledState(t *testing.T) {
	ctx := context.Background()
	store, err := state.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	registry := NewRegistry(store)
	manifest := validManifest("demo.agent", "0.2.0")
	manifest.Capabilities.Provides = []string{"demo.agent"}
	persistManifest(t, store, manifest, "registered")
	if err := registry.SetStatus(ctx, "demo.agent", "disabled", ""); err != nil {
		t.Fatal(err)
	}
	item, err := registry.Get(ctx, "demo.agent")
	if err != nil {
		t.Fatal(err)
	}
	if item.Status != "disabled" {
		t.Fatalf("status = %q, want disabled", item.Status)
	}
	if err := registry.SetStatus(ctx, "demo.agent", "restarting", ""); err == nil {
		t.Fatal("transient restarting status was accepted for persistence")
	}

	capabilities, err := registry.Capabilities(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if contains(capabilities, "demo.agent") {
		t.Fatalf("disabled module capability leaked: %#v", capabilities)
	}
}

func TestManifestAllowsWildcardSubscriptions(t *testing.T) {
	m := validManifest("monitoring", "1.0.0")
	m.Events.Subscribes = []string{"system.*", "job.*", "*"}
	m.Events.Publishes = []string{"monitoring.ready"}
	if err := ValidateManifest(m); err != nil {
		t.Fatalf("ValidateManifest() error = %v", err)
	}
}

func TestManifestRejectsForeignPublishNamespace(t *testing.T) {
	m := validManifest("monitoring", "1.0.0")
	m.Events.Publishes = []string{"system.updated"}
	if err := ValidateManifest(m); err == nil {
		t.Fatal("foreign event namespace was accepted")
	}
}

func TestPlanInstallRejectsBidirectionalConflict(t *testing.T) {
	base := validManifest("base", "1.0.0")
	feature := validManifest("feature", "1.0.0")
	base.Conflicts = []string{"feature"}

	_, err := PlanInstall(PlanInput{
		CoreVersion:  "0.1.0",
		Target:       "feature",
		Available:    map[string]Manifest{"base": base, "feature": feature},
		Installed:    map[string]string{"base": "1.0.0"},
		Capabilities: []string{"host.linux", "host.docker"},
		Architecture: "amd64",
	})
	if err == nil {
		t.Fatal("conflict declared by installed module was not detected")
	}
}

func TestRegistryCapabilitiesIncludeProvidedCapabilities(t *testing.T) {
	ctx := context.Background()
	store, err := state.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("state.Open() error = %v", err)
	}
	defer store.Close()

	registry := NewRegistry(store)
	manifest := validManifest("storage", "1.0.0")
	manifest.Capabilities.Provides = []string{"storage.block"}
	persistManifest(t, store, manifest, "registered")

	capabilities, err := registry.Capabilities(ctx)
	if err != nil {
		t.Fatalf("Capabilities() error = %v", err)
	}
	if !contains(capabilities, "storage.block") {
		t.Fatalf("provided capability missing: %#v", capabilities)
	}
}
