package modules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"sort"
	"strings"

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
	store               Store
	coreVersion         string
	registryCredentials func() (string, string)
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

func (r *Registry) SetRegistryCredentialsProvider(provider func() (string, string)) {
	r.registryCredentials = provider
}

func (r *Registry) InstallManifest(ctx context.Context, manifest Manifest) (Registered, error) {
	capabilities, err := r.Capabilities(ctx)
	if err != nil {
		return Registered{}, err
	}
	if err := CheckCompatibility(manifest, r.coreVersion, runtime.GOARCH, capabilities); err != nil {
		return Registered{}, err
	}

	raw, err := json.Marshal(manifest)
	if err != nil {
		return Registered{}, fmt.Errorf("encode module manifest: %w", err)
	}

	registryUser, registryToken := "", ""
	if r.registryCredentials != nil {
		registryUser, registryToken = r.registryCredentials()
	}
	if _, err := callModuleHelper(
		ctx,
		"docker.module.install",
		manifest.ID,
		manifest.Runtime.Docker.Image,
		manifest.Runtime.Health,
		registryUser,
		registryToken,
	); err != nil {
		return Registered{}, err
	}

	if err := r.store.UpsertModule(ctx, state.ModuleRecord{
		ID:           manifest.ID,
		Version:      manifest.Version,
		Status:       "enabled",
		ManifestJSON: string(raw),
	}); err != nil {
		return Registered{}, err
	}
	if err := r.store.SetModuleStatus(ctx, manifest.ID, "enabled", ""); err != nil {
		return Registered{}, err
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
	if _, err := callModuleHelper(ctx, helperOperation, id, "", HealthSpec{}, "", ""); err != nil {
		_ = r.store.SetModuleStatus(context.WithoutCancel(ctx), id, "error", err.Error())
		return Registered{}, err
	}
	if err := r.store.SetModuleStatus(ctx, id, nextStatus, ""); err != nil {
		return Registered{}, err
	}
	return r.Get(ctx, id)
}

func (r *Registry) Remove(ctx context.Context, id string, removeData ...bool) error {
	item, err := r.Get(ctx, id)
	if err != nil {
		return err
	}
	if item.Manifest.Runtime.Type != "docker" {
		return errors.New("module runtime is not Docker")
	}
	operation := "docker.module.remove"
	if len(removeData) > 0 && removeData[0] {
		operation = "docker.module.remove-data"
	}
	if _, err := callModuleHelper(ctx, operation, id, "", HealthSpec{}, "", ""); err != nil {
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
