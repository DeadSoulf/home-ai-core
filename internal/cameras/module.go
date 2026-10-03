package cameras

import (
	"context"
	"io"

	"github.com/DeadSoulf/home-ai-core/internal/modules"
)

const (
	ModuleID      = "cameras"
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
		Name:          "Cameras",
		Description:   "Hikvision and HiWatch cameras through the native HCNetSDK runtime",
		Version:       ModuleVersion,
		Core:          ">=0.1.152 <1.0.0",
		Permissions:   []string{"camera.manage"},
		Capabilities: modules.Capabilities{
			Requires: []string{"host.linux"},
			Provides: []string{"camera.hikvision", "camera.hcnetsdk"},
		},
		Host: modules.HostRequirements{
			Architectures: []string{"amd64"},
		},
		API: modules.APIContribution{Namespace: "cameras"},
		UI: modules.UIContract{Navigation: []modules.NavigationItem{{
			ID:    "cameras",
			Title: "Cameras",
			Route: "/modules/cameras",
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
