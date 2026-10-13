package modules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/state"
)

var (
	ErrModuleNotFound         = errors.New("module not found")
	ErrModuleAlreadyInstalled = errors.New("module already installed; automatic replacement is disabled until rollback-safe upgrades are implemented")
)

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
	mu          sync.Mutex
	helper      func(context.Context, string, string, string) (helperModuleResult, error)
}

func NewRegistry(store Store, coreVersion ...string) *Registry {
	version := ""
	if len(coreVersion) > 0 {
		version = strings.TrimSpace(coreVersion[0])
	}
	return &Registry{
		store:       store,
		coreVersion: version,
		helper:      callModuleHelper,
	}
}

func (r *Registry) InstallManifest(ctx context.Context, manifest Manifest) (Registered, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := ValidateManifest(manifest); err != nil {
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

	// Never overwrite the last known-good manifest before validating the
	// replacement container. Upgrades require a separate staged transaction.
	if _, err := r.store.Module(ctx, manifest.ID); err == nil {
		return Registered{}, ErrModuleAlreadyInstalled
	} else if !errors.Is(err, state.ErrModuleNotFound) {
		return Registered{}, err
	}

	raw, err := json.Marshal(manifest)
	if err != nil {
		return Registered{}, fmt.Errorf("encode module manifest: %w", err)
	}
	if _, err := r.helper(ctx, "docker.module.install", manifest.ID, manifest.Runtime.Docker.Image); err != nil {
		return Registered{}, err
	}
	if err := r.store.UpsertModule(ctx, state.ModuleRecord{
		ID:           manifest.ID,
		Version:      manifest.Version,
		Status:       "registered",
		ManifestJSON: string(raw),
	}); err != nil {
		return Registered{}, r.rollbackNewInstall(ctx, manifest.ID, err)
	}
	if err := r.store.SetModuleStatus(ctx, manifest.ID, "enabled", ""); err != nil {
		return Registered{}, r.rollbackNewInstall(ctx, manifest.ID, err)
	}
	return r.Get(ctx, manifest.ID)
}

// rollbackNewInstall compensates for a failure to persist a newly created
// container. The old registry state is never overwritten by the install path.
func (r *Registry) rollbackNewInstall(ctx context.Context, id string, cause error) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	if _, err := r.helper(cleanupCtx, "docker.module.remove", id, ""); err != nil {
		return errors.Join(cause, fmt.Errorf("remove unregistered module container %q: %w", id, err))
	}
	if err := r.store.DeleteModule(cleanupCtx, id); err != nil && !errors.Is(err, state.ErrModuleNotFound) {
		return errors.Join(cause, fmt.Errorf("remove incomplete module registration %q: %w", id, err))
	}
	return cause
}

func (r *Registry) Control(ctx context.Context, id, operation string) (Registered, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

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
	if _, err := r.helper(ctx, helperOperation, id, ""); err != nil {
		_ = r.store.SetModuleStatus(context.WithoutCancel(ctx), id, "error", err.Error())
		return Registered{}, err
	}
	if err := r.store.SetModuleStatus(ctx, id, nextStatus, ""); err != nil {
		return Registered{}, err
	}
	return r.Get(ctx, id)
}

func (r *Registry) Remove(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	item, err := r.Get(ctx, id)
	if err != nil {
		return err
	}
	if item.Manifest.Runtime.Type != "docker" {
		return errors.New("module runtime is not Docker")
	}
	if _, err := r.helper(ctx, "docker.module.remove", id, ""); err != nil {
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
	r.mu.Lock()
	defer r.mu.Unlock()

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
