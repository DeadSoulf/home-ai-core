# Changelog

## 0.1.211

- Added independent official Frigate integration, dynamic GitHub release discovery and verified per-architecture GHCR manifests.
- Require exact version/digest approval and fresh verification in both API and Job Engine; no automatic installation.
- Added fixed isolated Docker profile, real health checks, configuration/SQLite snapshot, automatic rollback and interrupted-upgrade recovery.
- Added native camera snapshots, events/clips, statistics and protected configuration editor.
- Removed redundant per-card update checks; keep the global check and upgrade actions only for available versions.
- Added real-container fault-injection tests, API/RBAC/CSRF tests, snapshot integrity and registry transaction tests.
- Updated source-map-js to 1.2.2; publish signed ordinary releases only after required verification, without draft releases.


## Unreleased

### 0.1.209-dev — native Home Assistant Container

- Added the reserved official Home Assistant module profile with explicit host-access confirmation, immutable images, platform/port validation and startup HTTP health checks.
- Connected catalog install/start/stop/restart/remove and manual upgrades to Core jobs; ordinary module sandbox restrictions remain unchanged.
- Added stable release discovery from upstream GitHub/GHCR with manifest-index digest verification and safe fallback to the pinned catalog entry.
- Upgrades use separate configuration generations so failed startup or registry persistence can restore the previous image AND original database. Removal preserves all configuration data.
- Added isolated lifecycle/recovery tests, manifest policy tests and frontend URL tests. See `docs/integrations/homeassistant-container.md` for privileges, supported platforms and recovery limitations.

### Core architecture reset

- Product-specific embedded runtimes have been removed from Home-AI-Core.
- Core is now defined as a trusted control plane rather than a bundle of product workloads.
- Module Registry and Module SDK remain as the foundation for independently distributed modules.
- Product modules will be delivered as separate Docker containers with independent versions and lifecycle.
- Core no longer carries module-specific runtime dependencies.
- Upgraded installations receive a cleanup migration that removes retired embedded product state.
- The next development stage is Docker Module Runtime.

Historical implementation notes for removed embedded prototypes are intentionally not retained in the active changelog.
