# Changelog

## 0.1.87-dev

- Added kernel-enforced free-space reserve protection for direct SMB writes on quota-ready Home-AI storage.
- Extended privileged helper protocol v3 so managed Samba shares carry their NAS pool root and reserve policy.
- Reused the existing `force user = home-ai-core` Samba boundary so filesystem user quotas cover every managed SMB write.
- Compute the service-UID hard limit from current Home-AI usage plus only live free space above the configured pool reserve, accounting for pre-existing data outside `.home-ai`.
- Verify quota usage and hard limits through `repquota` and apply changes through `setquota`.
- Make `smb.apply` fail closed before Samba configuration activation when a non-zero reserve cannot be kernel-enforced.
- Synchronize kernel quota best-effort whenever an administrator changes a pool reserve and expose explicit SMB hard-quota readiness/error state in the Files UI.
- Install Linux quota tools alongside Samba.
- Prepare newly created ext4 storage with embedded user quotas and no separate ext4 root reserve; mount ext4 with `usrquota` and XFS with `uquota`.
- Keep legacy ext4/XFS conversion non-destructive: existing filesystems are not automatically reformatted or live-remounted when quota accounting is unavailable.
- Added ADR-0034 plus quota calculation, parser and SMB metadata tests; PR #65 passed the full CI suite.

## 0.1.86-dev

- Added per-NAS-pool free-space capacity policies with migration 016.
- Existing and new pools default to a 5% hard reserve and a 10% low-space warning threshold.
- Read live filesystem capacity with Linux `statfs` and expose free/total, reserve/warning bytes and capacity state through the Files API.
- Added a Web editor for pool reserve and warning percentages with visible OK / low-space / reserve-reached states.
- Enforce the hard reserve for direct uploads before their temporary file can cross the configured limit.
- Reject resumable uploads that cannot fit above the reserve and re-check live free space before every chunk.
- Return HTTP 507 `file_pool_reserve_reached` for capacity-blocked Core/Windows-client writes and fail capacity-consuming API writes closed when capacity cannot be read.
- Keep same-filesystem moves, recycle-bin operations and restores available because they do not materially increase occupied bytes.
- Documented the SMB boundary: direct Samba writes still bypass Core and need a later filesystem/Samba quota layer for hard enforcement.
- Added ADR-0033 plus state, filedata, API and Web client coverage; PR #63 passed the full CI suite.

## 0.1.85-dev

- Hardened physical storage that backs active Home-AI file pools against accidental destructive operations.
- Storage-purpose responses now expose whether a device is in use and which file pools depend on it.
- Block purpose clearing/reassignment, unmount, formatting and partition deletion while the target storage backs a Home-AI file pool.
- Added explicit Web status for storage used by Files and disabled destructive controls while it is active.
- Added migration 015 so new NAS pools persist their backing device path and filesystem UUID.
- New file pools can only be created on the exact mounted filesystem explicitly assigned to the `files` purpose.
- Prefer filesystem UUID over Linux device path for pool ownership and usage locks, preserving protection across unmounts and `/dev/...` renumbering.
- Keep a mount-path fallback for pools created before migration 015.
- Added storage identity, legacy compatibility and reformatted-filesystem safety tests; full CI passed for PRs #60 and #61.

## 0.1.84-dev

- Added `client update` to discover the newest compatible Home-AI Windows client release automatically.
- Require the exact versioned amd64 `.exe` and `.sha256` release asset pair before an update is accepted.
- Bound release metadata, checksum and executable downloads and require HTTPS outside loopback-only tests.
- Verify the published SHA-256 before activating the cached executable and verify the cached file again afterward.
- Launch only the verified downloaded executable into the existing per-user `client install` handoff.
- Allow the bounded Windows replacement handoff to wait for the old parent process to exit while preserving tray-agent stop/restart behavior.
- Cache verified update payloads under the current user's Home-AI cache directory without storing credentials.
- Added ADR-0032, release/download tests, native Windows tests and full cross-platform CI coverage.

## 0.1.83-dev

- Unified Home-AI household identities across the Core instead of module-specific user models.
- Added household profiles: Administrator, Parent, Child, Guest and Friend.
- Administrator retains full access; the final enabled Administrator cannot be disabled or demoted.
- Added direct per-user global permissions in addition to existing role permissions.
- Added a central access catalog so the Users Web page can show and edit current Core capabilities.
- Added exact per-user resource grants for NAS file folders, with separate read/write access.
- Converted shared-folder access from the legacy member-role behavior to explicit per-user assignment while preserving existing grants during migration.
- Aligned SMB share ACL generation with the same effective Core permissions and scoped file-folder grants used by Web/API.
- Added protection against self-escalation and against assigning administrator-only user-management permissions to non-administrator profiles.
- Added migration 014, ADR-0031, RU/EN Web UI, security/state/API tests and full CI coverage.

## 0.1.82-dev

- Added Russian and English localization to the native Windows settings client in one executable.
- Added first-launch Windows locale detection: Russian locales default to Russian, other locales default to English.
- Added a live **Русский / English** selector and persisted the non-secret language preference in `windows-client.json`.
- Localized settings labels, buttons, conflict policies, profile status, client-side validation, confirmations and status messages.
- Localized tray menu, sync summary, tooltip and success/failure notifications using the same saved language.
- Refresh the running tray immediately when the language changes without restarting the sync scheduler.
- Keep passwords exclusively in Windows Credential Manager; language selection does not affect credentials or sync profiles.
- Keep low-level server/OS error details verbatim inside localized error framing; the diagnostic CLI remains English.
- Added ADR-0029, localization persistence/translation tests, native Windows tests and full cross-platform CI coverage.
## 0.1.81-dev

- Added a native Win32 settings window to the existing Windows client while preserving all CLI commands.
- Launching the Windows client without arguments now opens settings; Explorer-created private console windows are hidden.
- Added server/account setup, writable Home-AI folder discovery, a native local-folder picker, sync interval and conflict-policy controls.
- Added sync profile create/edit/enable/disable/delete; edits preserve profile identity, enabled state and previous sync result metadata.
- Keep passwords exclusively in Windows Credential Manager; the new client settings JSON stores only the canonical server URL and username.
- Added GUI controls for manual **Sync now** and background-agent/autostart management; running agents receive sync requests through the existing single scheduler.
- Added **Settings** to the tray menu and made tray double-click open the settings window.
- Scope discovered NAS folders to the connected server/account to prevent accidental reuse after changing connection details.
- Added ADR-0028, settings persistence tests, profile-edit tests, Windows-native tests and full cross-platform CI coverage.
## 0.1.80-dev

- Added a bounded Windows self-update handoff so `client install` and `agent install` can replace a stable installed executable while the tray agent is running.
- Added strict parsing of the Home-AI-managed HKCU Run command and restart of the updated agent without accepting arbitrary shell commands.
- Preserve the existing temp-file, durable write, atomic activation and SHA-256 verification path during executable replacement.
- Added tray health status showing enabled profile count and the latest `OK` / `FAILED` sync result.
- Added Windows notifications for failed sync cycles and manual **Sync now** completion while keeping successful scheduled cycles silent.
- Added ADR-0027, parser/status tests, and a native Windows test that replaces a genuinely locked executable through the tray `WM_CLOSE` handoff.

## 0.1.79-dev

- Added a stable per-user Windows client install path at `%LOCALAPPDATA%\\HomeAI\\bin\\home-ai-windows-client.exe`.
- Added `client install/status`; installation uses a temporary file, durable write, atomic replacement and SHA-256 verification.
- Changed `agent install` to register the stable installed executable instead of an arbitrary downloaded release path.
- Added a native Win32 notification-area tray without a third-party GUI dependency.
- Added tray actions for `Sync now`, opening the agent log, opening sync profiles, and clean agent exit.
- Routed manual tray sync through the existing sequential scheduler and single-instance agent rather than starting a second process.
- Added ADR-0026, stable-path/native Windows tests, and full core CI coverage.
## 0.1.78-dev

- Added a per-user Windows background sync agent with `agent install/status/remove/run` commands.
- Added current-user HKCU Run autostart after Windows logon; the startup command contains no password or bearer token.
- Require enabled sync profiles and matching Windows Credential Manager entries before enabling autostart.
- Added a single-instance OS lock, hidden Windows console runtime, and rotating per-user sync-agent log.
- Reuse the existing scheduled sync watcher and Credential Manager authentication without introducing a LocalSystem service.
- Added ADR-0025 and native Windows HKCU Run round-trip tests.

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
