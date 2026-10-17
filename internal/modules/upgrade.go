package modules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/state"
	"github.com/DeadSoulf/home-ai-core/internal/systeminfo"
)

// UpgradeManifest updates an already running NVR from a newer trusted catalog
// manifest. The helper keeps the stopped old container until the candidate's
// Docker health check succeeds and rolls back on candidate failure.
func (r *Registry) UpgradeManifest(ctx context.Context, candidate Manifest) (Registered, error) {
	if err := ctx.Err(); err != nil {
		return Registered{}, err
	}
	if !r.mu.TryLock() {
		return Registered{}, ErrModuleBusy
	}
	defer r.mu.Unlock()
	if err := ValidateManifest(candidate); err != nil {
		return Registered{}, err
	}
	// Only NVR has a verified backwards-compatible SQLite migration in this
	// release. Arbitrary third-party modules may perform destructive schema
	// migrations on startup and cannot be rolled back with container rollback.
	if candidate.ID != "nvr.module" {
		return Registered{}, errors.New("safe automatic upgrades are currently available only for NVR")
	}
	record, err := r.store.Module(ctx, candidate.ID)
	if err != nil {
		return Registered{}, err
	}
	if record.Status != "enabled" {
		return Registered{}, errors.New("module must be enabled before upgrading")
	}
	var previous Manifest
	if err := json.Unmarshal([]byte(record.ManifestJSON), &previous); err != nil {
		return Registered{}, err
	}
	oldVersion, err := parseVersion(previous.Version)
	if err != nil {
		return Registered{}, err
	}
	newVersion, err := parseVersion(candidate.Version)
	if err != nil || compareVersion(newVersion, oldVersion) <= 0 {
		return Registered{}, errors.New("new module version must be newer than the installed version")
	}
	if !contains(previous.Lifecycle, "upgrade") || !contains(candidate.Lifecycle, "upgrade") {
		return Registered{}, errors.New("the installed module does not support upgrade")
	}
	if r.coreVersion != "" {
		core := strings.TrimPrefix(r.coreVersion, "v")
		if p := strings.IndexAny(core, "+-"); p >= 0 {
			core = core[:p]
		}
		compatible, err := satisfies(core, candidate.Core)
		if err != nil || !compatible {
			return Registered{}, fmt.Errorf("new NVR requires Core %s", candidate.Core)
		}
	}
	// Existing images stay immutable; never reinterpret a mutable tag.
	if previous.Runtime.Docker.Image == candidate.Runtime.Docker.Image {
		return Registered{}, errors.New("upgrade must change the immutable image digest")
	}
	videoMount, err := r.resolveNVRVideoMount(ctx)
	if err != nil {
		return Registered{}, err
	}
	upgrade := r.upgradeHelper
	if upgrade == nil {
		upgrade = callModuleUpgradeHelper
	}
	if _, err := upgrade(ctx, candidate.ID, previous.Runtime.Docker.Image, candidate.Runtime.Docker.Image, videoMount); err != nil {
		return Registered{}, err
	}
	// The helper has already performed a verified swap. The DB update is
	// idempotent: a retry can adopt an already-upgraded managed container.
	raw, err := json.Marshal(candidate)
	if err != nil {
		return Registered{}, err
	}
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 8*time.Second)
	defer cancel()
	if err := r.store.UpsertModule(finishCtx, state.ModuleRecord{
		ID: candidate.ID, Version: candidate.Version, Status: "enabled", ManifestJSON: string(raw),
	}); err != nil {
		return Registered{}, fmt.Errorf("container upgraded, but registry update failed; inspect before retry: %w", err)
	}
	if err := r.store.SetModuleStatus(finishCtx, candidate.ID, "enabled", ""); err != nil {
		return Registered{}, fmt.Errorf("module upgraded but final status write failed: %w", err)
	}
	return r.Get(finishCtx, candidate.ID)
}

// AttachNVRStorage recreates the same pinned NVR image with the dedicated
// video directory mounted at /data/video. Its SQLite database and existing
// /data/recordings remain under the original protected module data root.
func (r *Registry) AttachNVRStorage(ctx context.Context) (Registered, error) {
	if !r.mu.TryLock() {
		return Registered{}, ErrModuleBusy
	}
	defer r.mu.Unlock()
	record, err := r.store.Module(ctx, "nvr.module")
	if err != nil {
		return Registered{}, err
	}
	var manifest Manifest
	if err := json.Unmarshal([]byte(record.ManifestJSON), &manifest); err != nil {
		return Registered{}, err
	}
	if record.Status != "enabled" {
		return Registered{}, errors.New("NVR must be enabled to attach video storage")
	}
	installedVersion, err := parseVersion(manifest.Version)
	if err != nil || compareVersion(installedVersion, version{major: 1, minor: 1, patch: 3}) < 0 {
		return Registered{}, errors.New("dedicated video storage requires NVR 1.1.3 or later")
	}
	mount, err := r.resolveNVRVideoMount(ctx)
	if err != nil {
		return Registered{}, err
	}
	if mount == "" {
		return Registered{}, errors.New("assign and mount a dedicated NVR partition first")
	}
	upgrade := r.upgradeHelper
	if upgrade == nil {
		upgrade = callModuleUpgradeHelper
	}
	if _, err := upgrade(ctx, manifest.ID, manifest.Runtime.Docker.Image, manifest.Runtime.Docker.Image, mount); err != nil {
		return Registered{}, err
	}
	return r.Get(ctx, manifest.ID)
}

func (r *Registry) resolveNVRVideoMount(ctx context.Context) (string, error) {
	if r.videoMount != nil {
		return r.videoMount(ctx)
	}
	source, ok := r.store.(interface {
		ListStoragePurposes(context.Context) ([]state.StoragePurposeRecord, error)
	})
	if !ok {
		return "", nil
	}
	records, err := source.ListStoragePurposes(ctx)
	if err != nil {
		return "", err
	}
	for _, record := range records {
		if record.Purpose != "module:nvr.module" {
			continue
		}
		nodes := systeminfo.Collect("").BlockTree
		node, ok := findNVRAssignedNode(nodes, record)
		if !ok {
			return "", errors.New("assigned NVR disk is unavailable; upgrade refused")
		}
		if node.System || (node.Filesystem != "ext4" && node.Filesystem != "xfs") {
			return "", errors.New("NVR video disk must be a non-system ext4 or xfs partition")
		}
		for _, mount := range node.Mountpoints {
			mount = filepath.Clean(mount)
			if filepath.Dir(mount) == "/mnt/home-ai-core" {
				return mount, nil
			}
		}
		return "", errors.New("mount the assigned NVR partition under /mnt/home-ai-core first")
	}
	return "", nil
}

func findNVRAssignedNode(nodes []systeminfo.BlockNode, assigned state.StoragePurposeRecord) (systeminfo.BlockNode, bool) {
	for _, node := range nodes {
		// A filesystem UUID is authoritative and must not fall back to a
		// potentially reused /dev/sdX path.
		if assigned.FilesystemUUID != "" {
			if node.UUID == assigned.FilesystemUUID {
				return node, true
			}
		} else if node.Path == assigned.DevicePath {
			return node, true
		}
		if child, ok := findNVRAssignedNode(node.Children, assigned); ok {
			return child, true
		}
	}
	return systeminfo.BlockNode{}, false
}
