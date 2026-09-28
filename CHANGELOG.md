# Changelog

## Unreleased

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
