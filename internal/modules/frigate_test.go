package modules

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/DeadSoulf/home-ai-core/internal/frigate"
	"github.com/DeadSoulf/home-ai-core/internal/state"
)

func frigateModuleFixture(version, letter string) Manifest {
	return FrigateManifest(frigate.Candidate{Version: version, Architecture: runtime.GOARCH, Image: frigate.ImageRepository + "@sha256:" + strings.Repeat(letter, 64)})
}
func TestFrigateManifestRequiresReviewedContract(t *testing.T) {
	valid := frigateModuleFixture("0.18.2", "a")
	if err := ValidateManifest(valid); err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []func(*Manifest){
		func(m *Manifest) {
			m.Runtime.Docker.Image = "ghcr.io/untrusted/frigate@sha256:" + strings.Repeat("a", 64)
		},
		func(m *Manifest) { m.Version = "0.19.0" }, func(m *Manifest) { m.Runtime.Health.Port = 8971 },
		func(m *Manifest) { m.Core = ">=0.1.0" }, func(m *Manifest) { m.API.Namespace = "nvr.module" },
		func(m *Manifest) { m.Host.Architectures = []string{"amd64", "arm64"} },
	} {
		m := frigateModuleFixture("0.18.2", "a")
		mutation(&m)
		if err := ValidateManifest(m); err == nil {
			t.Fatalf("invalid Frigate manifest accepted: %#v", m)
		}
	}
}
func TestFrigateUpgradeVerifiesAgainBeforeHelper(t *testing.T) {
	ctx := context.Background()
	db, err := state.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	old := frigateModuleFixture("0.18.2", "a")
	persistManifest(t, db, old, "enabled")
	registry := NewRegistry(db, "0.1.211")
	next := frigateModuleFixture("0.18.3", "b")
	verification := errors.New("approved digest no longer matches upstream")
	registry.verifyUpstream = func(context.Context, Manifest) error { return verification }
	registry.upgradeHelper = func(context.Context, string, string, string, string, func(string, string)) (helperModuleResult, error) {
		t.Fatal("helper executed before provenance verification")
		return helperModuleResult{}, nil
	}
	if _, err := registry.UpgradeManifest(ctx, next); !errors.Is(err, verification) {
		t.Fatalf("verification failure ignored: %v", err)
	}
	item, err := registry.Get(ctx, frigate.ModuleID)
	if err != nil || item.Manifest.Version != old.Version {
		t.Fatal("registry changed before verification", err)
	}
}
func TestFrigateRegistryCommitAndRollbackProtocol(t *testing.T) {
	for _, fail := range []bool{false, true} {
		ctx := context.Background()
		db, err := state.Open(ctx, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		old := frigateModuleFixture("0.18.2", "a")
		persistManifest(t, db, old, "enabled")
		wrapper := &failingUpgradeStore{Store: db, fail: fail}
		registry := NewRegistry(wrapper, "0.1.211")
		next := frigateModuleFixture("0.18.3", "b")
		verified := false
		var operations []string
		registry.verifyUpstream = func(_ context.Context, m Manifest) error {
			if m.Runtime.Docker.Image != next.Runtime.Docker.Image {
				t.Fatal("wrong image verified")
			}
			verified = true
			return nil
		}
		registry.upgradeHelper = func(_ context.Context, operation, id, image, previous string, _ func(string, string)) (helperModuleResult, error) {
			if !verified || id != frigate.ModuleID || image != next.Runtime.Docker.Image || previous != old.Runtime.Docker.Image {
				t.Fatal("unsafe helper arguments")
			}
			operations = append(operations, operation)
			return helperModuleResult{State: "running"}, nil
		}
		_, err = registry.UpgradeManifest(ctx, next)
		if fail {
			if err == nil || len(operations) != 2 || operations[1] != "docker.module.upgrade.rollback" {
				t.Fatalf("database failure not compensated: %v %v", err, operations)
			}
		} else if err != nil || len(operations) != 2 || operations[1] != "docker.module.upgrade.commit" {
			t.Fatalf("healthy candidate not committed: %v %v", err, operations)
		}
		item, getErr := registry.Get(ctx, frigate.ModuleID)
		if getErr != nil {
			t.Fatal(getErr)
		}
		expected := next.Version
		if fail {
			expected = old.Version
		}
		if item.Manifest.Version != expected {
			t.Fatal("wrong persisted version")
		}
		db.Close()
	}
}
