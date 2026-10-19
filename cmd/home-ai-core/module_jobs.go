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
		item, err := registry.InstallManifest(ctx, manifest, func(stage, detail string) {
			progress := 1500
			switch stage {
			case "prepare":
				progress = 900
			case "download", "build":
				progress = 1500
			case "downloaded":
				progress = 6000
			case "storage":
				progress = 7000
			case "create":
				progress = 8000
			case "start":
				progress = 8800
			case "verify":
				progress = 9200
			default:
				return
			}
			// This is a phase milestone, NOT an image-download percentage.
			// Details are sanitized at the privileged Docker helper boundary.
			if len(detail) > 110 {
				detail = detail[:110]
			}
			message := "module.phase." + stage
			if detail != "" {
				message += "|" + detail
			}
			_ = reporter.Progress(ctx, progress, message)
		})
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

    if err := service.Register("module.upgrade", func(ctx context.Context, job state.JobRecord, reporter jobs.Reporter) (map[string]any, error) {
        raw, ok := job.Input["manifest_json"].(string)
        if !ok || raw == "" { return nil, errors.New("module upgrade manifest missing") }
        manifest, err := modules.DecodeManifest(strings.NewReader(raw))
        if err != nil { return nil, err }
        _ = reporter.Progress(ctx, 500, "validating module upgrade")
        item, err := registry.UpgradeManifest(ctx, manifest, func(stage, detail string) {
            progress := 1500
            switch stage {
            case "download": progress = 1500
            case "create": progress = 6800
            case "start": progress = 8000
            case "verify": progress = 8900
            case "cleanup_warning": progress = 9400
            }
            message := "module.phase." + stage
            if detail != "" { message += ":" + detail }
            _ = reporter.Progress(ctx, progress, message)
        })
        if err != nil { return nil, err }
        _ = reporter.Progress(ctx, 9800, "AI module upgraded; persistent data preserved")
        return map[string]any{"module_id": item.Manifest.ID, "status": item.Status, "version": item.Manifest.Version}, nil
    }); err != nil { return err }

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
		_ = reporter.Progress(ctx, 2000, "removing module container")
		if err := registry.Remove(ctx, moduleID); err != nil {
			return nil, err
		}
		_ = reporter.Progress(ctx, 9500, "module removed; persistent data preserved")
		return map[string]any{
			"module_id":      moduleID,
			"data_preserved": true,
		}, nil
	}); err != nil {
		return err
	}

	if err := service.Register("module.gpu.prepare", func(ctx context.Context, job state.JobRecord, reporter jobs.Reporter) (map[string]any, error) {
		if job.Input["module_id"] != "ai.agent" {
			return nil, errors.New("unsupported GPU module")
		}
		_ = reporter.Progress(ctx, 1500, "module.phase.download")
		err := modules.PrepareAIGPUImage(ctx, func(stage, detail string) {
			progress := 1500
			if stage == "downloaded" {
				progress = 9500
			}
			if len(detail) > 110 {
				detail = detail[:110]
			}
			message := "module.phase." + stage
			if detail != "" {
				message += "|" + detail
			}
			_ = reporter.Progress(ctx, progress, message)
		})
		if err != nil {
			return nil, err
		}
		_ = reporter.Progress(ctx, 9500, "module.phase.downloaded")
		return map[string]any{"module_id": "ai.agent", "status": "gpu_image_cached"}, nil
	}); err != nil {
		return err
	}

	return nil
}
