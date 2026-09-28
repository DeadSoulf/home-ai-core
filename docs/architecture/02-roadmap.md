# Development Roadmap

## Phase 0 — Repository reset
- Preserve previous implementation in an archive branch.
- Replace the active working tree with a clean project skeleton.
- Recreate `develop` from the new baseline.

## Phase 1 — Specification
- Product scope and boundaries.
- Core responsibilities.
- Module vs App vs Integration definitions.
- Security model.
- API conventions.
- Third-party code policy.

## Phase 2 — Repository structure
- Core, web, SDK, schemas, packaging and tests layout.
- ADR process.
- Git workflow.
- Versioning and release rules.

## Phase 3 — Core v0.1
- Daemon/bootstrap.
- Configuration.
- Logging.
- Health endpoint.
- Read-only system information.

## Phase 4 — Core API
- Versioned REST API.
- WebSocket event channel.
- Error model.
- API documentation.

## Phase 5 — Security
- Users and sessions.
- RBAC and permissions.
- Module capability grants.
- Audit log.

## Phase 6 — Jobs and Events
- Persistent job model.
- Progress and cancellation.
- Typed event bus.
- WebSocket delivery.

## Phase 7 — Module SDK
- Manifest schema.
- Dependency model.
- Permissions.
- Lifecycle: install, upgrade, remove, backup, restore.
- UI registration.

## Phase 8 — Web UI Core
- Dashboard.
- System.
- Modules.
- Jobs.
- Audit.
- Settings.

## Phase 9 — Module Store
- Repository metadata.
- Install/update/remove.
- Compatibility validation.
- Signed packages.
- Rollback.

## Phase 10 — Containers
- Docker Engine.
- Compose/stacks.
- Images, networks, volumes and registries.

## Phase 11 — Storage and NAS
- Block devices, SMART, filesystems and mounts.
- ZFS/Btrfs/LVM providers where available.
- SMB/NFS shares.

## Phase 12 — Virtualization
- libvirt/KVM/QEMU.
- VM lifecycle.
- ISO/images, networks, storage, PCI/USB/GPU passthrough.

## Phase 13 — Backup
- Core configuration.
- Modules.
- Apps.
- VM and application data.

## Phase 14 — Network and VPN
- Host networking.
- Bridges and VLANs.
- VPN integrations.

## Phase 15 — AI
- GPU discovery.
- Model/runtime management.
- AI applications.

## Phase 16 — Apps
- App manifests.
- Docker/Compose based application catalog.
- Application backup and restore.

## Phase 17 — Installer
- Supported clean Debian 13 installation path.
- Upgrade and recovery tooling.

## Phase 18 — 1.0 release
- CI and full test matrix.
- Security review.
- Backup/restore validation.
- Documentation.
