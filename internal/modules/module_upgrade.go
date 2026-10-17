package modules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/state"
)

var (
	ErrModuleUpgradeUnavailable = errors.New("module is not installed and enabled for upgrade")
	ErrModuleUpgradeVersion = errors.New("safe in-place upgrade currently supports patch versions within the same major/minor only")
)

// UpgradeManifest is a strictly version-forward lifecycle operation. The
// helper transaction preserves data and restores the old container if the
// candidate fails to become healthy. Core commits the manifest only after
// the helper reports a healthy new container.
func (r *Registry) UpgradeManifest(ctx context.Context, manifest Manifest) (Registered, error) {
	if ctx.Err() != nil {
		return Registered{}, ctx.Err()
	}
	if !r.mu.TryLock() {
		return Registered{}, ErrModuleBusy
	}
	defer r.mu.Unlock()
	if err := ValidateManifest(manifest); err != nil {
		return Registered{}, err
	}
	if r.coreVersion != "" {
		current := strings.TrimPrefix(r.coreVersion, "v")
		if pos := strings.IndexAny(current, "+-"); pos >= 0 { current = current[:pos] }
		if _, err := parseVersion(current); err == nil {
			ok, err := satisfies(current, manifest.Core)
			if err != nil { return Registered{}, err }
			if !ok {
				return Registered{}, fmt.Errorf("module %q is incompatible with Core %s", manifest.ID, r.coreVersion)
			}
		}
	}
	record, err := r.store.Module(ctx, manifest.ID)
	if errors.Is(err, state.ErrModuleNotFound) {
		return Registered{}, ErrModuleUpgradeUnavailable
	}
	if err != nil {
		return Registered{}, err
	}
	if record.Status != "enabled" {
		return Registered{}, ErrModuleUpgradeUnavailable
	}
	var previous Manifest
	if err := json.Unmarshal([]byte(record.ManifestJSON), &previous); err != nil {
		return Registered{}, fmt.Errorf("decode installed manifest: %w", err)
	}
	oldVersion, err := parseVersion(previous.Version)
	if err != nil { return Registered{}, err }
	newVersion, err := parseVersion(manifest.Version)
	if err != nil { return Registered{}, err }
	if newVersion.major != oldVersion.major || newVersion.minor != oldVersion.minor ||
		compareVersion(newVersion, oldVersion) <= 0 ||
		manifest.Runtime.Type != previous.Runtime.Type ||
		manifest.Runtime.Docker.Image == previous.Runtime.Docker.Image {
		return Registered{}, ErrModuleUpgradeVersion
	}
	raw, err := json.Marshal(manifest)
	if err != nil { return Registered{}, err }
	helper := r.upgradeHelper
	if helper == nil { helper = callModuleUpgradeHelper }
	if _, err := helper(ctx, manifest.ID, previous.Runtime.Docker.Image, manifest.Runtime.Docker.Image); err != nil {
		saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = r.store.SetModuleStatus(saveCtx, manifest.ID, record.Status, "upgrade failed; previous module retained or restored: "+err.Error())
		return Registered{}, err
	}
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := r.store.UpsertModule(finishCtx, state.ModuleRecord{
		ID: manifest.ID, Version: manifest.Version,
		Status: "enabled", ManifestJSON: string(raw),
	}); err != nil {
		return Registered{}, fmt.Errorf("module upgraded in Docker but registry update failed; retry the exact upgrade manifest: %w", err)
	}
	if err := r.store.SetModuleStatus(finishCtx, manifest.ID, "enabled", ""); err != nil {
		return Registered{}, fmt.Errorf("module upgraded; status persistence failed: %w", err)
	}
	return r.Get(finishCtx, manifest.ID)
}
