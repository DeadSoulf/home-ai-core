package modules

import (
	"context"
	"errors"
	"runtime"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/frigate"
)

const FrigateCoreConstraint = ">=0.1.211 <1.0.0"

// Frigate uses a fixed, explicitly reviewed helper profile. It cannot request
// arbitrary host mounts, ports, devices, privileges or shell commands.
func FrigateManifest(c frigate.Candidate) Manifest {
	return Manifest{
		SchemaVersion: 2, ID: frigate.ModuleID, Name: "Frigate NVR",
		Description: "Official Frigate container; direct GitHub release discovery, verified platform digest, manual upgrades and configuration/database rollback. Independent of the Hikvision NVR module.",
		Version:     c.Version, Core: FrigateCoreConstraint,
		Runtime:      RuntimeSpec{Type: "docker", Docker: DockerSpec{Image: c.Image}, Health: HealthSpec{Port: 5000, Path: "/api/version"}},
		Capabilities: Capabilities{Requires: []string{"host.docker"}, Provides: []string{"frigate.nvr"}},
		Host:         HostRequirements{Architectures: []string{c.Architecture}}, API: APIContribution{Namespace: frigate.ModuleID},
		UI:        UIContract{Navigation: []NavigationItem{{ID: frigate.ModuleID, Title: "Frigate", Route: "/modules/frigate.module", Icon: "video", Order: 30}}},
		Lifecycle: []string{"install", "upgrade", "remove", "start", "stop", "restart"},
	}
}

func validateFrigateManifest(m Manifest) error {
	if m.ID != frigate.ModuleID {
		return nil
	}
	if !frigate.ValidImage(m.Runtime.Docker.Image) || !frigate.CompatibleVersion(m.Version) {
		return errors.New("Frigate requires an official immutable image and a supported stable version")
	}
	if m.Core != FrigateCoreConstraint || m.Runtime.Health.Port != 5000 || m.Runtime.Health.Path != "/api/version" || m.API.Namespace != frigate.ModuleID {
		return errors.New("Frigate manifest does not match the reviewed Core integration contract")
	}
	if len(m.Host.Architectures) != 1 || (m.Host.Architectures[0] != "amd64" && m.Host.Architectures[0] != "arm64") {
		return errors.New("Frigate image must pin exactly one supported Linux architecture")
	}
	return nil
}

// Reverify inside the job, not just on the HTTP submission path. A cached
// catalog or a queued job cannot authorize a tag that changed after approval.
func verifyOfficialFrigate(ctx context.Context, m Manifest) error {
	if m.ID != frigate.ModuleID {
		return nil
	}
	if err := validateFrigateManifest(m); err != nil {
		return err
	}
	if runtime.GOOS != "linux" || m.Host.Architectures[0] != runtime.GOARCH {
		return errors.New("Frigate image is incompatible with this host")
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	candidate, err := frigate.NewClient(nil).Resolve(ctx, m.Version, runtime.GOARCH)
	if err != nil {
		return err
	}
	if candidate.Image != m.Runtime.Docker.Image {
		return errors.New("official Frigate digest changed after approval; refresh and review the new image")
	}
	return nil
}
