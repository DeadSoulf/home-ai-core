package modules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/DeadSoulf/home-ai-core/internal/state"
)

var ErrModuleNotFound = errors.New("module not found")

type Store interface {
	UpsertModule(context.Context, state.ModuleRecord) error
	Module(context.Context, string) (state.ModuleRecord, error)
	ListModules(context.Context) ([]state.ModuleRecord, error)
	SetModuleStatus(context.Context, string, string, string) error
}

type Registry struct {
	store Store

	mu      sync.RWMutex
	modules map[string]Module
}

func NewRegistry(store Store) *Registry {
	return &Registry{
		store:   store,
		modules: make(map[string]Module),
	}
}

func (r *Registry) Register(ctx context.Context, module Module) error {
	if module == nil {
		return errors.New("module is nil")
	}
	manifest := module.Manifest()
	if err := ValidateManifest(manifest); err != nil {
		return err
	}

	raw, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("encode module manifest: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.modules[manifest.ID]; exists {
		return fmt.Errorf("module %q already registered", manifest.ID)
	}

	if err := r.store.UpsertModule(ctx, state.ModuleRecord{
		ID:           manifest.ID,
		Version:      manifest.Version,
		Status:       "registered",
		ManifestJSON: string(raw),
	}); err != nil {
		return err
	}
	r.modules[manifest.ID] = module
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

func (r *Registry) Runtime(id string) (Module, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	module, ok := r.modules[id]
	return module, ok
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
