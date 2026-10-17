package modules

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DeadSoulf/home-ai-core/internal/state"
)

func nvrUpgradeFixtures(t *testing.T) (*state.Store, *Registry, Manifest, Manifest) {
	t.Helper()
	ctx := context.Background()
	db, err := state.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	previous := validManifest("nvr.module", "1.1.2")
	previous.Lifecycle = append(previous.Lifecycle, "upgrade")
	previous.Core = ">=0.1.167 <1.0.0"
	persistManifest(t, db, previous, "enabled")
	candidate := previous
	candidate.Version = "1.1.3"
	candidate.Runtime.Docker.Image = "ghcr.io/deadsoulf/home-ai-core_dvr@sha256:" + strings.Repeat("b", 64)
	registry := NewRegistry(db, "0.1.187-dev")
	registry.videoMount = func(context.Context) (string, error) { return "", nil }
	return db, registry, previous, candidate
}

func TestUpgradeNVRChangesManifestOnlyAfterSuccessfulDockerHealth(t *testing.T) {
	ctx := context.Background()
	db, registry, previous, candidate := nvrUpgradeFixtures(t)
	defer db.Close()
	calls := 0
	registry.upgradeHelper = func(_ context.Context, id, previousImage, nextImage, mount string) (helperModuleResult, error) {
		calls++
		if id != "nvr.module" || previousImage != previous.Runtime.Docker.Image ||
			nextImage != candidate.Runtime.Docker.Image || mount != "" {
			t.Fatalf("unexpected NVR image transition %s %s -> %s (%s)", id, previousImage, nextImage, mount)
		}
		// Docker is healthy at this point.
		return helperModuleResult{State: "running"}, nil
	}
	result, err := registry.UpgradeManifest(ctx, candidate)
	if err != nil { t.Fatal(err) }
	if calls != 1 || result.Status != "enabled" || result.Manifest.Version != candidate.Version {
		t.Fatalf("bad NVR upgrade %#v, calls %d", result, calls)
	}
	stored, err := db.Module(ctx, "nvr.module")
	if err != nil || stored.Version != candidate.Version || !strings.Contains(stored.ManifestJSON, candidate.Runtime.Docker.Image) {
		t.Fatalf("registry not updated after health: %#v %v", stored, err)
	}
}

func TestUpgradeFailedHelperLeavesOriginalModuleRegistration(t *testing.T) {
	ctx := context.Background()
	db, registry, previous, candidate := nvrUpgradeFixtures(t)
	defer db.Close()
	registry.upgradeHelper = func(context.Context, string, string, string, string) (helperModuleResult, error) {
		return helperModuleResult{}, errors.New("candidate unhealthy; rollback succeeded")
	}
	if _, err := registry.UpgradeManifest(ctx, candidate); err == nil {
		t.Fatal("expected failed upgrade")
	}
	stored, err := db.Module(ctx, previous.ID)
	if err != nil || stored.Version != previous.Version || stored.Status != "enabled" ||
		!strings.Contains(stored.ManifestJSON, previous.Runtime.Docker.Image) {
		t.Fatalf("failed upgrade modified persisted module %#v %v", stored, err)
	}
}

func TestUpgradeRejectsUnsupportedOrDowngradeBeforeDocker(t *testing.T) {
	db, registry, previous, candidate := nvrUpgradeFixtures(t)
	defer db.Close()
	calls := 0
	registry.upgradeHelper = func(context.Context, string, string, string, string) (helperModuleResult, error) {
		calls++
		return helperModuleResult{State: "running"}, nil
	}
	for _, bad := range []Manifest{
		previous,
		func() Manifest {m := candidate; m.Version = "1.1.1"; return m}(),
		func() Manifest {m := candidate; m.ID = "other"; return m}(),
		func() Manifest {m := candidate; m.Lifecycle = []string{"install"}; return m}(),
		func() Manifest {m := candidate; m.Runtime.Docker.Image = previous.Runtime.Docker.Image; return m}(),
	} {
		if _, err := registry.UpgradeManifest(context.Background(), bad); err == nil {
			t.Fatalf("unsafe upgrade accepted: %+v", bad)
		}
	}
	if calls != 0 { t.Fatalf("unsafe upgrade reached Docker %d times", calls) }
}

func TestAttachNVRVideoStorageRejectsOlderImageAndMissingDisk(t *testing.T) {
	db, registry, _, candidate := nvrUpgradeFixtures(t)
	defer db.Close()
	registry.videoMount = func(context.Context) (string, error) { return "/mnt/home-ai-core/video/nvr-video", nil }
	registry.upgradeHelper = func(context.Context, string, string, string, string) (helperModuleResult, error) {
		t.Fatal("old NVR must not attach a disk")
		return helperModuleResult{}, nil
	}
	if _, err := registry.AttachNVRStorage(context.Background()); err == nil {
		t.Fatal("old NVR should not accept dedicated video disk")
	}
	persistManifest(t, db, candidate, "enabled")
	registry.videoMount = func(context.Context) (string, error) { return "", nil }
	if _, err := registry.AttachNVRStorage(context.Background()); err == nil {
		t.Fatal("mounting NVR without assigned disk accepted")
	}
	registry.videoMount = func(context.Context) (string, error) { return "/mnt/home-ai-core/video", nil }
	calls := 0
	registry.upgradeHelper = func(_ context.Context, id, previousImage, nextImage, mount string) (helperModuleResult, error) {
		calls++
		if id != "nvr.module" || previousImage != candidate.Runtime.Docker.Image ||
			nextImage != candidate.Runtime.Docker.Image || mount != "/mnt/home-ai-core/video" {
			t.Fatalf("bad disk reconfigure %s %s %s %s", id, previousImage, nextImage, mount)
		}
		return helperModuleResult{State:"running"}, nil
	}
	result, err := registry.AttachNVRStorage(context.Background())
	if err != nil || result.Status != "enabled" || calls != 1 {
		t.Fatalf("attach result %#v %v", result, err)
	}
}
