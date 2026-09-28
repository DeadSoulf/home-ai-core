package demo

import (
	"context"
	"io"

	"github.com/DeadSoulf/home-ai-core/internal/modules"
)

type Module struct{}

func New() *Module { return &Module{} }

func (m *Module) Manifest() modules.Manifest {
	return modules.Manifest{
		SchemaVersion: modules.ManifestSchemaVersion,
		ID:            "core-demo",
		Name:          "Core Demo Module",
		Description:   "Reference module used to validate the Module SDK contract.",
		Version:       "0.1.0",
		Core:          ">=0.1.0 <1.0.0",
		Permissions:   []string{"system.read"},
		Capabilities: modules.Capabilities{
			Requires: []string{"host.linux"},
			Provides: []string{"demo.reference"},
		},
		Host: modules.HostRequirements{
			Architectures: []string{"amd64", "arm64"},
		},
		API: modules.APIContribution{Namespace: "core-demo"},
		Events: modules.EventContract{
			Publishes: []string{"demo.ready"},
		},
		UI: modules.UIContract{
			Navigation: []modules.NavigationItem{
				{ID: "overview", Title: "Demo", Route: "/modules/core-demo"},
			},
		},
		Lifecycle: []string{"install", "upgrade", "remove", "backup", "restore"},
	}
}

func (m *Module) Lifecycle() modules.Lifecycle { return lifecycle{} }

type lifecycle struct{}

func (lifecycle) Install(context.Context, modules.OperationContext, modules.Progress) error {
	return nil
}
func (lifecycle) Upgrade(context.Context, modules.OperationContext, modules.Progress) error {
	return nil
}
func (lifecycle) Remove(context.Context, modules.OperationContext, modules.Progress) error {
	return nil
}
func (lifecycle) Backup(context.Context, modules.OperationContext, modules.Progress, io.Writer) error {
	return nil
}
func (lifecycle) Restore(context.Context, modules.OperationContext, modules.Progress, io.Reader) error {
	return nil
}
