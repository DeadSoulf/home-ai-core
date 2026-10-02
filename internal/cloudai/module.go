package cloudai

import (
	"context"
	"io"

	"github.com/DeadSoulf/home-ai-core/internal/modules"
)

type Module struct{}

func NewModule() Module { return Module{} }

func (Module) Manifest() modules.Manifest {
	return modules.Manifest{
		SchemaVersion: modules.ManifestSchemaVersion,
		ID:            "ai.cloud",
		Name:          "Cloud AI",
		Description:   "Optional OpenAI-compatible cloud provider for the Home-AI agent",
		Version:       "0.1.0",
		Core:          ">=0.1.131 <1.0.0",
		Dependencies: []modules.Dependency{{
			ID:      "ai.agent",
			Version: ">=0.8.0 <1.0.0",
		}},
		Capabilities: modules.Capabilities{
			Requires: []string{"host.linux", "ai.agent"},
			Provides: []string{"ai.cloud-provider"},
		},
		API: modules.APIContribution{Namespace: "ai.cloud"},
		UI: modules.UIContract{Navigation: []modules.NavigationItem{{
			ID:    "cloud",
			Title: "Cloud AI",
			Route: "/modules/ai.cloud",
			Order: 25,
		}}},
		Lifecycle: []string{"backup", "restore"},
	}
}

func (Module) Lifecycle() modules.Lifecycle { return moduleLifecycle{} }

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
