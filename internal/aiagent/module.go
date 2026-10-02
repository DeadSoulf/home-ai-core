package aiagent

import (
	"context"
	"io"

	"github.com/DeadSoulf/home-ai-core/internal/modules"
)

type Module struct{}

func NewModule() Module {
	return Module{}
}

func (Module) Manifest() modules.Manifest {
	return modules.Manifest{
		SchemaVersion: modules.ManifestSchemaVersion,
		ID:            "ai.agent",
		Name:          "AI Agent",
		Description:   "First-party Home-AI agent orchestration foundation",
		Version:       "0.3.0",
		Core:          ">=0.1.0 <1.0.0",
		Permissions:   []string{"jobs.read", "modules.read", "system.read"},
		Capabilities: modules.Capabilities{
			Requires: []string{"host.linux"},
			Provides: []string{"ai.agent", "ai.chat", "ai.tools"},
		},
		API: modules.APIContribution{Namespace: "ai.agent"},
		Events: modules.EventContract{
			Publishes: []string{"ai.agent.tool.called", "ai.agent.tool.completed"},
		},
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
