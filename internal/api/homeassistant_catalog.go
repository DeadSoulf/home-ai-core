package api

import (
	"context"
	"net/http"

	"github.com/DeadSoulf/home-ai-core/internal/homeassistant"
	"github.com/DeadSoulf/home-ai-core/internal/modules"
)

// Only an explicitly published official catalog entry opts in to this profile.
// Upstream failure preserves the known immutable catalog image and does not
// hide other modules. Refresh is read-only: installation/upgrades remain jobs
// explicitly requested by an authorized operator.
func refreshHomeAssistantCandidate(ctx context.Context, catalog *moduleCatalogDocument, transport http.RoundTripper) {
	for i := range catalog.Modules {
		if catalog.Modules[i].ID != homeassistant.ModuleID {
			continue
		}
		release, err := homeassistant.ResolveLatest(ctx, transport)
		if err != nil {
			catalog.HomeAssistantStatus = "upstream_unavailable"
			return
		}
		if release.Version == catalog.Modules[i].Version {
			if release.Image != catalog.Modules[i].Runtime.Docker.Image {
				catalog.HomeAssistantStatus = "digest_changed"
				return
			}
			catalog.HomeAssistantStatus = "up_to_date"
			return
		}
		if !newerNVRSemver(release.Version, catalog.Modules[i].Version) {
			catalog.HomeAssistantStatus = "catalog_newer"
			return
		}
		candidate := catalog.Modules[i]
		candidate.Version = release.Version
		candidate.Runtime.Docker.Image = release.Image
		candidate.Host.Architectures = release.Architectures
		if err := modules.ValidateManifest(candidate); err != nil {
			catalog.HomeAssistantStatus = "upstream_unavailable"
			return
		}
		catalog.Modules[i] = candidate
		catalog.HomeAssistantStatus = "available"
		return
	}
}
