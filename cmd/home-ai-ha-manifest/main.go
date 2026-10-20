// home-ai-ha-manifest resolves a stable official Home Assistant release to an
// immutable catalog manifest. It does not install, update or publish anything.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/DeadSoulf/home-ai-core/internal/homeassistant"
	"github.com/DeadSoulf/home-ai-core/internal/modules"
)

func main() {
	if len(os.Args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: home-ai-ha-manifest (writes verified manifest JSON to stdout)")
		os.Exit(2)
	}
	release, err := homeassistant.ResolveLatest(context.Background(), nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	manifest := modules.Manifest{SchemaVersion: 2, ID: homeassistant.ModuleID, Name: "Home Assistant", Version: release.Version, Core: homeassistant.MinimumCore,
		Description:  "Official Home Assistant Container. Dedicated host-network/privileged profile with persistent configuration, HTTP health checks and copy-isolated manual upgrades. No Supervisor or apps. Requires explicit host-access approval.",
		Runtime:      modules.RuntimeSpec{Type: "docker", Docker: modules.DockerSpec{Image: release.Image}, Health: modules.HealthSpec{Port: 8123, Path: "/"}},
		Permissions:  []string{"host.network", "host.privileged"},
		Capabilities: modules.Capabilities{Requires: []string{"host.docker"}, Provides: []string{"homeassistant.service"}},
		Host:         modules.HostRequirements{Architectures: release.Architectures}, API: modules.APIContribution{Namespace: homeassistant.ModuleID},
		Lifecycle: []string{"install", "upgrade", "start", "stop", "restart", "remove"}}
	if err := modules.ValidateManifest(manifest); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(manifest); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
