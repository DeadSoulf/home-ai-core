# Changelog

## 0.1.77-dev

- Added Windows Credential Manager storage for Home-AI passwords without putting secrets in queue/sync JSON or command-line arguments.
- Added `credentials save/status/delete` CLI commands; `HOME_AI_PASSWORD` remains the explicit highest-priority one-shot override.
- Added Credential Manager fallback for Windows `folders`, copy, queue and scheduled sync authentication after process restart.
- Added native Windows tests that round-trip a real generic credential and authenticate the Home-AI client without an environment password.
- Added ADR-0024 and clarified that the future background launcher must run in the user's logon session rather than as LocalSystem.

## 0.1.76-dev

- Added persistent Windows push-sync profiles with explicit intervals and enable/disable state; profiles store no password or bearer token.
- Added `sync add/list/run/watch/enable/disable/remove` CLI commands and fresh source rescanning on every sync run.
- Added explicit destination conflict policies: `stop`, `skip` and recoverable `replace-to-trash` using the existing Home-AI recycle bin.
- Refuse automatic replacement of conflicting ancestor paths, and keep this first sync mode additive: local deletions are not mirrored to the server.
- Added ADR-0023, Windows-native tests and full core CI coverage for scheduled sync behavior.

## 0.1.75-dev

- Added recursive Windows file-client copying, including empty directories and safe creation of missing destination directories.
- Added a persistent local transfer queue with add/list/run/retry commands, process locking, atomic saves and recovery after interruption.
- Preserve completed work and verify queued source snapshots and existing destination checksums before treating a transfer as complete; credentials remain process-only.
- Commit resumable server uploads atomically without replacing a concurrently created target.
- Documented the queue contract, usage and remaining live NAS/SMB acceptance work.

## 0.1.74-dev

- Grouped Web navigation into Server, Services and Management, with a compact mobile drawer and keyboard support.
- Split System into Equipment, Storage, Network and Updates while preserving file, user, storage, network and update controls.
- Simplified Home to essential server status, warnings and recent activity; moved identifiers and diagnostic metadata into expandable details.
- Show readable operation and security-event labels while retaining progress, failures and troubleshooting details.
- Filter navigation and management controls by the current user's permissions.
- Show a downloaded, verified update as complete instead of an active 100% operation.


## Unreleased

### Repository cleanup and reliability audit

- Removed the unused legacy `internal/coreupdate` version-comparison package.
- Removed the unregistered Core demo module and empty top-level scaffold directories.
- Simplified routine CI so Debian packages are built only by the explicit initial-installer workflow.
- Removed a redundant updater-helper build from the update-release workflow.
- Switched update-status reads to the dedicated `updates.read` permission.
- Removed an unused storage inspection wrapper.
- Hardened storage unmount handling for devices mounted at multiple targets and added socket deadline margin.
- Refreshed updater, storage, runtime, packaging and repository-layout documentation to match the implemented architecture.
- Verified routine CI with dependency lock checks, gofmt, schema/shell checks, Web typecheck/tests/build, Go tests/vet, integration smoke test and amd64/arm64 cross-builds.

### Current platform work


- Enable and start the privileged Core update helper automatically during Debian package installation.

- Restarted project architecture from a clean working tree.
- Preserved the previous implementation in `archive/pre-restart-2026-09-28`.
- Defined the Home-AI-Core v1 product scope, module boundaries, multi-node direction, AI trust model and future mobile-sync requirements.
- Selected Go for the control plane and TypeScript/React for the Web UI.
- Defined an unprivileged public Core with a separate future privileged helper boundary.
- Added SQLite control-plane state with embedded transactional migrations and persistent node registration.
- Added read-only Linux hardware discovery for CPU, memory, block devices, network interfaces and basic GPU/PCI inventory.
- Added `/health` and `/api/v1/system` Core API endpoints with request correlation IDs.
- Added Debian/systemd runtime contracts and baseline service hardening.
- Added locked Go dependencies, unit tests, vet, integration daemon smoke tests and Linux amd64/arm64 cross-build validation.
- Defined Core API v1 error/correlation contracts and added realtime WebSocket Event Envelope v1 with subscriptions, heartbeat and reconnect semantics.
- Added first-owner bootstrap, Argon2id password hashing, persistent sessions, RBAC permissions, CSRF protection and security audit records.
- Added persistent typed jobs, cooperative cancellation, durable event history and cursor-backed realtime delivery.
- Added Module SDK v1 with strict manifests, dependency/conflict planning, lifecycle contracts, capability discovery and persistent read-only module registry APIs.
- Added Phase 8A Web UI foundation with first-run/login flows, Dashboard/System/Modules/Jobs/Audit pages, realtime refresh and same-origin static serving from Core.
- Added Debian 13 installation packages for amd64/arm64, packaged-artifact smoke tests and physical-server installation tooling.

- Added Russian Web UI localization with RU/EN switching, browser-language detection and saved preference.
- Started Phase 9 with signed module repository metadata and Ed25519/SHA-256 package verification.
- Added driver-independent PCI GPU inventory with model resolution and Web UI driver status; defined automatic hardware driver reconciliation through the future privileged helper.
- Added the Debian pci.ids hardware database as a package dependency for reliable human-readable PCI device names.
- Added Web-driven Core update discovery and installation with persistent jobs, SHA-256 staging verification, automatic reconnect and a constrained root update helper.
- Use release-safe `.dev` asset filenames while preserving Debian `~dev` package version ordering so Web update manifests match GitHub release assets exactly.
- Added a 0.1.7-dev acceptance release to validate the first real Web-driven Core upgrade from 0.1.6-dev.
