package modules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/state"
)

var ErrModuleNotFound = errors.New("module not found")

type Store interface {
	UpsertModule(context.Context, state.ModuleRecord) error
	Module(context.Context, string) (state.ModuleRecord, error)
	ListModules(context.Context) ([]state.ModuleRecord, error)
	SetModuleStatus(context.Context, string, string, string) error
	DeleteModule(context.Context, string) error
}

type Registry struct {
	store       Store
	coreVersion string
}

func NewRegistry(store Store, coreVersion ...string) *Registry {
	version := ""
	if len(coreVersion) > 0 {
		version = strings.TrimSpace(coreVersion[0])
	}
	return &Registry{
		store:       store,
		coreVersion: version,
	}
}

func (r *Registry) InstallManifest(ctx context.Context, manifest Manifest) (Registered, error) {
	if err := ValidateManifest(manifest); err != nil {
		return Registered{}, err
	}
	if _, err := r.store.Module(ctx, manifest.ID); err == nil {
		return Registered{}, fmt.Errorf("module %q is already registered; use the verified upgrade operation", manifest.ID)
	} else if !errors.Is(err, state.ErrModuleNotFound) {
		return Registered{}, err
	}
	if r.coreVersion != "" {
		current := strings.TrimPrefix(r.coreVersion, "v")
		if idx := strings.IndexAny(current, "+-"); idx >= 0 {
			current = current[:idx]
		}
		if _, err := parseVersion(current); err == nil {
			ok, err := satisfies(current, manifest.Core)
			if err != nil {
				return Registered{}, err
			}
			if !ok {
				return Registered{}, fmt.Errorf("module %q is incompatible with Core %s", manifest.ID, r.coreVersion)
			}
		}
	}

	raw, err := json.Marshal(manifest)
	if err != nil {
		return Registered{}, fmt.Errorf("encode module manifest: %w", err)
	}
	if err := r.store.UpsertModule(ctx, state.ModuleRecord{
		ID:           manifest.ID,
		Version:      manifest.Version,
		Status:       "registered",
		ManifestJSON: string(raw),
	}); err != nil {
		return Registered{}, err
	}
	if err := r.store.SetModuleStatus(ctx, manifest.ID, "registered", ""); err != nil {
		return Registered{}, err
	}

	if _, err := callModuleHelper(ctx, "docker.module.install", manifest.ID, manifest.Runtime.Docker.Image); err != nil {
		_ = r.store.SetModuleStatus(context.WithoutCancel(ctx), manifest.ID, "error", err.Error())
		return Registered{}, err
	}
	if err := r.store.SetModuleStatus(ctx, manifest.ID, "enabled", ""); err != nil {
		return Registered{}, err
	}
	return r.Get(ctx, manifest.ID)
}

// UpgradeManifest keeps the old registry record until the helper has started and
// health-checked the replacement. The helper keeps the old container as a
// rollback candidate until the registry commit succeeds.
func (r *Registry) UpgradeManifest(ctx context.Context, manifest Manifest) (Registered, error) {
	if err := ValidateManifest(manifest); err != nil {
		return Registered{}, err
	}
	previous, err := r.Get(ctx, manifest.ID)
	if err != nil {
		return Registered{}, err
	}
	if previous.Status != "enabled" {
		return Registered{}, errors.New("stop or failed module cannot be upgraded; module must be enabled")
	}
	newer, err := NewerVersion(manifest.Version, previous.Manifest.Version)
	if err != nil {
		return Registered{}, err
	}
	if !newer {
		return Registered{}, fmt.Errorf("module %q version %s is not newer than installed version %s", manifest.ID, manifest.Version, previous.Manifest.Version)
	}
	if manifest.Runtime.Health.Port == 0 || manifest.Runtime.Health.Path == "" {
		return Registered{}, errors.New("module update requires an explicit HTTP health endpoint")
	}

	coreVersion := strings.TrimPrefix(r.coreVersion, "v")
	if idx := strings.IndexAny(coreVersion, "+-"); idx >= 0 {
		coreVersion = coreVersion[:idx]
	}
	if _, err := parseVersion(coreVersion); err != nil {
		return Registered{}, fmt.Errorf("cannot validate module upgrade against Core version %q: %w", r.coreVersion, err)
	}
	all, err := r.List(ctx)
	if err != nil {
		return Registered{}, err
	}
	installed := map[string]string{}
	available := map[string]Manifest{manifest.ID: manifest}
	for _, item := range all {
		installed[item.Manifest.ID] = item.Manifest.Version
		if item.Manifest.ID != manifest.ID {
			available[item.Manifest.ID] = item.Manifest
		}
	}
	capabilities, err := r.Capabilities(ctx)
	if err != nil {
		return Registered{}, err
	}
	if _, err := PlanInstall(PlanInput{
		CoreVersion: coreVersion, Target: manifest.ID,
		Available: available, Installed: installed,
		Capabilities: capabilities,
	}); err != nil {
		return Registered{}, fmt.Errorf("module update compatibility: %w", err)
	}

	oldImage := previous.Manifest.Runtime.Docker.Image
	if _, err := callModuleUpgradeHelper(ctx, manifest.ID, manifest.Runtime.Docker.Image, oldImage, manifest.Runtime.Health); err != nil {
		return Registered{}, err
	}
	raw, err := json.Marshal(manifest)
	if err == nil {
		err = r.store.UpsertModule(ctx, state.ModuleRecord{
			ID: manifest.ID, Version: manifest.Version,
			Status: "enabled", ManifestJSON: string(raw),
		})
	}
	if err != nil {
		// The registry must never claim a new version after a failed commit.
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		if _, rollbackErr := callModuleHelper(rollbackCtx, "docker.module.rollback", manifest.ID, ""); rollbackErr != nil {
			return Registered{}, errors.Join(err, fmt.Errorf("Docker rollback failed: %w", rollbackErr))
		}
		return Registered{}, fmt.Errorf("module registry commit failed; prior container restored: %w", err)
	}
	if _, err := callModuleHelper(ctx, "docker.module.finalize", manifest.ID, ""); err != nil {
		// The stopped previous container is intentionally preserved for recovery.
		_ = r.store.SetModuleStatus(context.WithoutCancel(ctx), manifest.ID, "enabled", "previous container cleanup pending: "+err.Error())
	}
	return r.Get(ctx, manifest.ID)
}

func (r *Registry) Control(ctx context.Context, id, operation string) (Registered, error) {
	item, err := r.Get(ctx, id)
	if err != nil {
		return Registered{}, err
	}
	if item.Manifest.Runtime.Type != "docker" {
		return Registered{}, errors.New("module runtime is not Docker")
	}

	helperOperation := ""
	nextStatus := ""
	switch operation {
	case "enable":
		helperOperation = "docker.module.start"
		nextStatus = "enabled"
	case "disable":
		helperOperation = "docker.module.stop"
		nextStatus = "disabled"
	case "restart":
		helperOperation = "docker.module.restart"
		nextStatus = "enabled"
	default:
		return Registered{}, fmt.Errorf("unsupported module control operation %q", operation)
	}
	if _, err := callModuleHelper(ctx, helperOperation, id, ""); err != nil {
		_ = r.store.SetModuleStatus(context.WithoutCancel(ctx), id, "error", err.Error())
		return Registered{}, err
	}
	if err := r.store.SetModuleStatus(ctx, id, nextStatus, ""); err != nil {
		return Registered{}, err
	}
	return r.Get(ctx, id)
}

func (r *Registry) Remove(ctx context.Context, id string) error {
	item, err := r.Get(ctx, id)
	if err != nil {
		return err
	}
	if item.Manifest.Runtime.Type != "docker" {
		return errors.New("module runtime is not Docker")
	}
	if _, err := callModuleHelper(ctx, "docker.module.remove", id, ""); err != nil {
		_ = r.store.SetModuleStatus(context.WithoutCancel(ctx), id, "error", err.Error())
		return err
	}
	if err := r.store.DeleteModule(ctx, id); err != nil {
		if errors.Is(err, state.ErrModuleNotFound) {
			return ErrModuleNotFound
		}
		return err
	}
	return nil
}

func (r *Registry) Get(ctx context.Context, id string) (Registered, error) {
	record, err := r.store.Module(ctx, id)
	if errors.Is(err, state.ErrModuleNotFound) {
		return Registered{}, ErrModuleNotFound
	}
	if err != nil {
		return Registered{}, err
	}
	return registeredFromRecord(record)
}

func (r *Registry) List(ctx context.Context) ([]Registered, error) {
	records, err := r.store.ListModules(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Registered, 0, len(records))
	for _, record := range records {
		item, err := registeredFromRecord(record)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Manifest.ID < out[j].Manifest.ID })
	return out, nil
}

func (r *Registry) SetStatus(ctx context.Context, id, status, errorMessage string) error {
	switch status {
	case "registered", "enabled", "disabled", "error":
	default:
		return fmt.Errorf("invalid module status %q", status)
	}
	if _, err := r.Get(ctx, id); err != nil {
		return err
	}
	if err := r.store.SetModuleStatus(ctx, id, status, errorMessage); err != nil {
		if errors.Is(err, state.ErrModuleNotFound) {
			return ErrModuleNotFound
		}
		return err
	}
	return nil
}

func registeredFromRecord(record state.ModuleRecord) (Registered, error) {
	var manifest Manifest
	if err := json.Unmarshal([]byte(record.ManifestJSON), &manifest); err != nil {
		return Registered{}, fmt.Errorf("decode registered manifest %q: %w", record.ID, err)
	}
	return Registered{
		Manifest: manifest,
		Status:   record.Status,
		Error:    record.Error,
	}, nil
}
