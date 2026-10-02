package nvr

import (
	"context"
	"io"

	"github.com/DeadSoulf/home-ai-core/internal/modules"
)

const (
	ModuleID      = "nvr"
	ModuleVersion = "0.1.0"
)

type Module struct{}

func NewModule() Module {
	return Module{}
}

func (Module) Manifest() modules.Manifest {
	return modules.Manifest{
		SchemaVersion: modules.ManifestSchemaVersion,
		ID:            ModuleID,
		Name:          "Cameras / NVR",
		Description:   "Local-first camera, live view and recording foundation",
		Version:       ModuleVersion,
		Core:          ">=0.1.134 <1.0.0",
		Capabilities: modules.Capabilities{
			Requires: []string{"host.linux"},
			Provides: []string{"camera.nvr"},
		},
		Host: modules.HostRequirements{
			Architectures: []string{"amd64", "arm64"},
			Packages:      []string{"ffmpeg"},
		},
		API: modules.APIContribution{Namespace: "nvr"},
		Events: modules.EventContract{
			Publishes: []string{
				"nvr.camera.online",
				"nvr.camera.offline",
				"nvr.recording.started",
				"nvr.recording.stopped",
				"nvr.motion.started",
				"nvr.motion.ended",
				"nvr.storage.warning",
				"nvr.storage.full",
				"nvr.evidence.protected",
			},
		},
		UI: modules.UIContract{Navigation: []modules.NavigationItem{{
			ID:    "cameras",
			Title: "Cameras",
			Route: "/modules/nvr",
			Order: 20,
		}}},
		Lifecycle: []string{"backup", "restore"},
	}
}

func (Module) Lifecycle() modules.Lifecycle {
	return moduleLifecycle{}
}

type moduleLifecycle struct{}

func (moduleLifecycle) Install(context.Context, modules.OperationContext, modules.Progress) error {
	return nil
}

func (moduleLifecycle) Upgrade(context.Context, modules.OperationContext, modules.Progress) error {
	return nil
}

func (moduleLifecycle) Remove(context.Context, modules.OperationContext, modules.Progress) error {
	return nil
}

func (moduleLifecycle) Backup(context.Context, modules.OperationContext, modules.Progress, io.Writer) error {
	return nil
}

func (moduleLifecycle) Restore(context.Context, modules.OperationContext, modules.Progress, io.Reader) error {
	return nil
}
