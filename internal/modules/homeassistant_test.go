package modules

import (
	"strings"
	"testing"

	"github.com/DeadSoulf/home-ai-core/internal/homeassistant"
)

func TestHomeAssistantManifestRequiresExplicitFixedProfile(t *testing.T) {
	manifest := Manifest{SchemaVersion: 2, ID: homeassistant.ModuleID, Name: "Home Assistant", Version: "2026.10.2", Core: homeassistant.MinimumCore,
		Runtime:      RuntimeSpec{Type: "docker", Docker: DockerSpec{Image: homeassistant.Repository + "@sha256:" + strings.Repeat("a", 64)}, Health: HealthSpec{Port: 8123, Path: "/"}},
		Capabilities: Capabilities{Requires: []string{"host.docker"}}, Permissions: []string{"host.network", "host.privileged"},
		Host: HostRequirements{Architectures: []string{"amd64", "arm64"}}, Lifecycle: []string{"install", "upgrade", "start", "stop", "restart", "remove"}}
	if err := ValidateManifest(manifest); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*Manifest){
		func(m *Manifest) { m.Permissions = nil },
		func(m *Manifest) { m.Permissions = []string{"host.network"} },
		func(m *Manifest) {
			m.Runtime.Docker.Image = "ghcr.io/attacker/homeassistant@sha256:" + strings.Repeat("a", 64)
		},
		func(m *Manifest) { m.Runtime.Docker.Image = homeassistant.Repository + ":stable" },
		func(m *Manifest) { m.Runtime.Health.Port = 8080 },
		func(m *Manifest) { m.Runtime.Health.Path = "/api/" },
	} {
		candidate := manifest
		change(&candidate)
		if err := ValidateManifest(candidate); err == nil {
			t.Fatalf("unsafe Home Assistant manifest accepted: %#v", candidate)
		}
	}
}
