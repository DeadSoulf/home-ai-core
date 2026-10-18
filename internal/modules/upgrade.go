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

const aiUpgradeID = "ai.agent"

func (r *Registry) compatibleModule(ctx context.Context, manifest Manifest) error {
	if err := ValidateManifest(manifest); err != nil {
		return err
	}
	if r.coreVersion == "" {
		return nil
	}
	current := strings.TrimPrefix(r.coreVersion, "v")
	if i := strings.IndexAny(current, "+-"); i >= 0 {
		current = current[:i]
	}
	if _, err := parseVersion(current); err != nil {
		return err
	}
	ok, err := satisfies(current, manifest.Core)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("module %q is incompatible with Core %s", manifest.ID, r.coreVersion)
	}
	return nil
}

func validateAIUpgrade(previous, candidate Manifest, status string) error {
	if previous.ID != aiUpgradeID || candidate.ID != aiUpgradeID {
		return errors.New("rollback-safe upgrades are currently supported only for AI Agent")
	}
	if status != "enabled" {
		return errors.New("AI Agent must be enabled before an in-place update")
	}
	old, err := parseVersion(previous.Version)
	if err != nil {
		return err
	}
	next, err := parseVersion(candidate.Version)
	if err != nil {
		return err
	}
	if old.major != 0 || old.minor != 3 || next.major != 0 || next.minor != 3 || compareVersion(next, old) <= 0 {
		return errors.New("only newer compatible AI Agent 0.3.x patch versions can be updated automatically")
	}
	if candidate.Runtime.Type != "docker" || candidate.Runtime.Health.Port != 8080 || candidate.Runtime.Health.Path != "/health" {
		return errors.New("AI update needs the fixed Docker /health endpoint")
	}
	if previous.Runtime.Docker.Image == candidate.Runtime.Docker.Image {
		return errors.New("new AI module version must use a different immutable image")
	}
	if candidate.Host.Architectures == nil {
		return errors.New("AI module upgrade must declare supported architectures")
	}
	return nil
}

// CanUpgradeAI reports whether the current compatible patch update can be
// offered through the official catalog without promising support for NVR or
// third-party data formats.
func CanUpgradeAI(current Registered, candidate Manifest) bool {
	return validateAIUpgrade(current.Manifest, candidate, current.Status) == nil &&
		ValidateManifest(candidate) == nil
}

// UpgradeManifest is only used for official catalog manifests. The privileged
// helper snapshots persistent /data before booting the candidate and preserves
// the old container until this store has durably committed the new manifest.
func (r *Registry) UpgradeManifest(ctx context.Context, manifest Manifest, progress func(string, string)) (Registered, error) {
	if err := ctx.Err(); err != nil {
		return Registered{}, err
	}
	if !r.mu.TryLock() {
		return Registered{}, ErrModuleBusy
	}
	defer r.mu.Unlock()
	if err := r.compatibleModule(ctx, manifest); err != nil {
		return Registered{}, err
	}
	previous, err := r.store.Module(ctx, manifest.ID)
	if err != nil {
		return Registered{}, err
	}
	var old Manifest
	if err := json.Unmarshal([]byte(previous.ManifestJSON), &old); err != nil {
		return Registered{}, err
	}
	if err := validateAIUpgrade(old, manifest, previous.Status); err != nil {
		return Registered{}, err
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		return Registered{}, err
	}
	upgrader := r.upgradeHelper
	if upgrader == nil {
		upgrader = callAIUpgradeHelper
	}
	_, err = upgrader(ctx, "docker.module.upgrade", old.Runtime.Docker.Image,
		manifest.Runtime.Docker.Image, previous.ManifestJSON, progress)
	if err != nil {
		return Registered{}, err
	}
	// The caller may disconnect after the helper switched containers.
	// Complete DB commit with an independent bounded context.
	commitCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	persistErr := r.store.UpsertModule(commitCtx, state.ModuleRecord{
		ID: manifest.ID, Version: manifest.Version, ManifestJSON: string(raw), Status: "enabled",
	})
	if persistErr == nil {
		persistErr = r.store.SetModuleStatus(commitCtx, manifest.ID, "enabled", "")
	}
	if persistErr != nil {
		recovery, recoveryCancel := context.WithTimeout(context.Background(), 4*time.Minute)
		defer recoveryCancel()
		_, rollbackErr := upgrader(recovery, "docker.module.upgrade.rollback",
			old.Runtime.Docker.Image, old.Runtime.Docker.Image, previous.ManifestJSON, nil)
		if rollbackErr == nil {
			// Restore prior registry record if the first DB write succeeded.
			if err := r.store.UpsertModule(recovery, previous); err != nil {
				rollbackErr = err
			}
			if err := r.store.SetModuleStatus(recovery, previous.ID, previous.Status, previous.Error); err != nil {
				rollbackErr = errors.Join(rollbackErr, err)
			}
		}
		return Registered{}, errors.Join(fmt.Errorf("AI registry commit failed: %w", persistErr), rollbackErr)
	}
	finalCtx, finalCancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer finalCancel()
	_, err = upgrader(finalCtx, "docker.module.upgrade.finalize", old.Runtime.Docker.Image,
		manifest.Runtime.Docker.Image, previous.ManifestJSON, nil)
	if err != nil {
		return Registered{}, fmt.Errorf("AI upgrade committed; backup cleanup pending recovery: %w", err)
	}
	return r.Get(finalCtx, manifest.ID)
}

// RecoverPendingAIUpgrade runs before serving requests or recovering jobs.
// The registry image is the source of truth if the helper/Core crashed between
// Docker promotion and durable registry commit.
func (r *Registry) RecoverPendingAIUpgrade(ctx context.Context) error {
	if !r.mu.TryLock() {
		return ErrModuleBusy
	}
	defer r.mu.Unlock()
	record, err := r.store.Module(ctx, aiUpgradeID)
	if errors.Is(err, state.ErrModuleNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	var current Manifest
	if err := json.Unmarshal([]byte(record.ManifestJSON), &current); err != nil {
		return err
	}
	upgrader := r.upgradeHelper
	if upgrader == nil {
		upgrader = callAIUpgradeHelper
	}
	recovery, stop := context.WithTimeout(ctx, 5*time.Minute)
	defer stop()
	stateResult, err := upgrader(recovery, "docker.module.upgrade.reconcile", "",
		current.Runtime.Docker.Image, "", nil)
	if err != nil {
		return fmt.Errorf("AI pending upgrade recovery failed; refusing unsafe start: %w", err)
	}
	switch stateResult.State {
	case "none", "committed":
		return nil
	case "rolled_back":
		if stateResult.PreviousManifest == "" {
			return errors.New("AI recovery omitted previous manifest")
		}
		var old Manifest
		if err := json.Unmarshal([]byte(stateResult.PreviousManifest), &old); err != nil {
			return err
		}
		if old.ID != aiUpgradeID {
			return errors.New("invalid recovered AI module ID")
		}
		if err := ValidateManifest(old); err != nil {
			return err
		}
		if err := r.store.UpsertModule(recovery, state.ModuleRecord{
			ID: old.ID, Version: old.Version, Status: "enabled", ManifestJSON: stateResult.PreviousManifest,
		}); err != nil {
			return err
		}
		return r.store.SetModuleStatus(recovery, old.ID, "enabled", "")
	default:
		return fmt.Errorf("unexpected AI recovery status %q", stateResult.State)
	}
}
