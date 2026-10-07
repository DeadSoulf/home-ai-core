package main

import (
	"context"
	"errors"
	"strings"

	"github.com/DeadSoulf/home-ai-core/internal/jobs"
	"github.com/DeadSoulf/home-ai-core/internal/modules"
	"github.com/DeadSoulf/home-ai-core/internal/state"
)

func registerModuleJobs(service *jobs.Service, registry *modules.Registry) error {
	if service == nil || registry == nil {
		return errors.New("module job dependencies are required")
	}

	if err := service.Register("module.install", func(ctx context.Context, job state.JobRecord, reporter jobs.Reporter) (map[string]any, error) {
		raw, _ := job.Input["manifest_json"].(string)
		if strings.TrimSpace(raw) == "" {
			return nil, errors.New("module manifest is missing")
		}
		manifest, err := modules.DecodeManifest(strings.NewReader(raw))
		if err != nil {
			return nil, err
		}
		_ = reporter.Progress(ctx, 500, "validating module manifest")
		_ = reporter.Progress(ctx, 1500, "pulling and creating module container")
		item, err := registry.InstallManifest(ctx, manifest)
		if err != nil {
			return nil, err
		}
		_ = reporter.Progress(ctx, 9500, "module container is running")
		return map[string]any{
			"module_id": item.Manifest.ID,
			"version":   item.Manifest.Version,
			"status":    item.Status,
		}, nil
	}); err != nil {
		return err
	}

	if err := service.Register("module.control", func(ctx context.Context, job state.JobRecord, reporter jobs.Reporter) (map[string]any, error) {
		moduleID, _ := job.Input["module_id"].(string)
		operation, _ := job.Input["operation"].(string)
		if moduleID == "" || operation == "" {
			return nil, errors.New("module control input is incomplete")
		}
		_ = reporter.Progress(ctx, 1500, "applying module runtime operation")
		item, err := registry.Control(ctx, moduleID, operation)
		if err != nil {
			return nil, err
		}
		_ = reporter.Progress(ctx, 9500, "module runtime state updated")
		return map[string]any{
			"module_id": moduleID,
			"operation": operation,
			"status":    item.Status,
		}, nil
	}); err != nil {
		return err
	}

	if err := service.Register("module.remove", func(ctx context.Context, job state.JobRecord, reporter jobs.Reporter) (map[string]any, error) {
		moduleID, _ := job.Input["module_id"].(string)
		if moduleID == "" {
			return nil, errors.New("module id is missing")
		}
		removeData, _ := job.Input["remove_data"].(bool)
		_ = reporter.Progress(ctx, 2000, "removing module container")
		if err := registry.Remove(ctx, moduleID, removeData); err != nil {
			return nil, err
		}
		message := "module removed; persistent data preserved"
		if removeData {
			message = "module and persistent data removed"
		}
		_ = reporter.Progress(ctx, 9500, message)
		return map[string]any{
			"module_id":      moduleID,
			"data_preserved": !removeData,
		}, nil
	}); err != nil {
		return err
	}

	return nil
}
