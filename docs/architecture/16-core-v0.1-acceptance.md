# Core v0.1 Acceptance Criteria

Core Foundation v0.1 is considered complete when all items below are satisfied.

## Process

- [x] Go control-plane executable exists.
- [x] Public Core refuses to run as root.
- [x] Graceful SIGTERM/SIGINT shutdown exists.
- [x] Structured JSON logging exists.
- [x] Default listener is loopback-only.

## Identity and state

- [x] Stable local node ID exists.
- [x] SQLite Core state exists.
- [x] Embedded transactional migrations exist.
- [x] Local node is registered in Core state.
- [x] Database readiness participates in health status.

## Read-only discovery

- [x] OS/kernel information.
- [x] CPU information.
- [x] memory information.
- [x] block-device discovery.
- [x] network-interface discovery.
- [x] basic GPU/PCI discovery.

## API

- [x] `GET /health`.
- [x] `GET /api/v1/system`.
- [x] request correlation ID.
- [x] versioned API namespace.

## Debian runtime

- [x] dedicated service user contract.
- [x] systemd unit.
- [x] state/runtime/config directory contract.
- [x] baseline systemd hardening.

## Validation

- [x] unit tests.
- [x] `go vet`.
- [x] reproducible Go module lock files.
- [x] integration smoke test starts the real daemon.
- [x] Linux amd64 build.
- [x] Linux arm64 cross-build.

## Deferred to later phases

The following are intentionally **not** Core v0.1 requirements:

- authentication/users
- privileged helper implementation
- jobs/events
- Module SDK
- Web UI
- Docker
- NAS/storage mutation
- KVM
- smart home
- cameras
- AI runtime
- clustering
