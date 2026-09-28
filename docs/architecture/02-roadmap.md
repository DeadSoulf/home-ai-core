# Development Roadmap

## Phase 0 — Repository reset
- Preserve previous implementation in an archive branch.
- Replace the active working tree with a clean project skeleton.
- Recreate `develop` from the new baseline.

## Phase 1 — Product and architecture specification
- Product mission and supported home workloads.
- Core vs Module vs App vs Integration boundaries.
- Single-node and future multi-node resource identity.
- AI trust and permission model.
- Mobile sync requirements.
- Third-party code policy.

## Phase 2 — Technology and repository foundation
- Select Core implementation language/runtime.
- Select Web UI stack.
- Define IPC/privilege boundary.
- Core, web, SDK, schemas, packaging and tests layout.
- ADR process.
- Git workflow.
- Versioning and release rules.

## Phase 3 — Core v0.1
- Daemon/bootstrap.
- Configuration.
- Structured logging.
- Health endpoint.
- Node identity.
- Read-only host and capability discovery.

## Phase 4 — Core API and realtime transport
- Versioned REST API.
- WebSocket event channel.
- Stable resource IDs.
- Error model.
- Request correlation IDs.
- API documentation.

## Phase 5 — Identity and security
- Users and sessions.
- Device identities.
- RBAC and capability permissions.
- Module permission grants.
- Secrets handling.
- Audit log.
- AI actor identity and policy boundary.

## Phase 6 — Jobs and Events
- Persistent job model.
- Progress and cancellation.
- Typed event bus.
- WebSocket delivery.
- Node-aware event metadata.

## Phase 7 — Module SDK
- Manifest schema.
- Dependency and conflict model.
- Permissions.
- Lifecycle: install, upgrade, remove, backup, restore.
- UI registration.
- Capability discovery.

## Phase 8 — Web UI Core
- First-run setup.
- Dashboard.
- System.
- Users/devices.
- Modules.
- Jobs.
- Audit.
- Settings.

## Phase 9 — Module Store and Updates
- Repository metadata.
- Signed packages.
- Install/update/remove.
- Compatibility validation.
- Core/module update orchestration.
- Rollback.
- Hardware driver reconciliation using trusted signed profiles.
- Automatic driver jobs for newly detected PCI/USB hardware through the privileged helper.

## Phase 10 — Containers
- Docker Engine integration.
- Compose/stacks.
- Images, networks, volumes and registries.
- Application runtime abstraction.

## Phase 11 — Storage and Personal Cloud foundation
- Block devices, SMART, filesystems and mounts.
- ZFS/Btrfs/LVM providers where appropriate.
- Storage pools and datasets/volumes.
- User/family file spaces.
- File/media metadata API.
- SMB/NFS as optional NAS capabilities.
- Integrity and quota model.

## Phase 12 — Backup and Recovery
- Core configuration backup.
- Module and App state.
- User files and media.
- Off-host backup targets.
- Restore testing.
- Disaster recovery procedures.

## Phase 13 — Secure Networking and Remote Access
- Host networking.
- Bridges and VLANs.
- Service exposure policy.
- TLS and certificates.
- VPN/tunnel integrations.
- Secure remote access without exposing SMB/NFS directly.

## Phase 14 — Smart Home
- Home Assistant and/or equivalent integration.
- Device/state bridge into the Home-AI event bus.
- Permission-scoped actions.
- Automation triggers and notifications.

## Phase 15 — Video Surveillance
- Camera discovery/integration.
- RTSP/ONVIF-oriented ingest adapters.
- Live view.
- Recording and retention.
- Storage policies.
- Event model for motion/detections.
- Optional accelerator-backed video analytics.

## Phase 16 — AI Runtime
- GPU/accelerator discovery.
- Model/runtime management.
- Local LLM and multimodal inference endpoints.
- Resource limits and scheduling hints.
- Model storage and lifecycle.

## Phase 17 — AI Orchestration
- AI access to approved Core/Module tools.
- Permission and approval policies.
- System diagnostics.
- Assisted configuration.
- Smart-home orchestration.
- Search/reasoning over explicitly authorized private data.
- Full auditability of AI actions.

## Phase 18 — Multi-node Foundation
- Secure node enrollment.
- Node identity and trust.
- Capability/resource advertisement.
- Cluster health.
- Node-aware jobs/events.
- Storage and workload locality metadata.

## Phase 19 — Distributed Scheduling
- Placement of movable workloads.
- CPU/RAM/GPU resource accounting.
- Affinity and node-bound resources.
- Failure/degraded-mode behaviour.
- Optional distributed AI workload coordination.
- Multiple-server management from one UI.

## Phase 20 — Virtualization
- libvirt/KVM/QEMU.
- VM lifecycle.
- ISO/images.
- Virtual networks and storage.
- PCI/USB/GPU passthrough.
- Node-aware placement where supported.

## Phase 21 — Mobile Sync Server API
- Device registration.
- Resumable/chunked uploads.
- Automatic photo/video ingest API.
- Document synchronization.
- Content hashes and duplicate detection.
- Selective sync.
- Notification hooks.
- Per-device revocation and permissions.

## Phase 22 — Native Mobile Applications
- Android client.
- iOS client.
- Background camera roll backup.
- Documents and offline access.
- Secure credential storage.
- Local-network and remote operation.

Mobile applications should live in dedicated repositories once their API contract is stable.

## Phase 23 — Production 1.0
- CI and full test matrix.
- Security review.
- Upgrade/rollback validation.
- Backup/restore validation.
- Multi-node failure tests.
- Documentation.
- Supported Debian 13 installation and recovery tooling.
