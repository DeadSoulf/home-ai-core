# Home-AI-Core

Home-AI-Core is a modular private home-server control plane for Debian 13.

The platform is designed to manage personal infrastructure from one Web UI while keeping the network-facing Core unprivileged. Privileged host operations are isolated in a narrowly scoped root helper.

## Current platform

The repository currently includes:

- Go control-plane daemon and React/TypeScript Web UI
- first-run owner bootstrap and authenticated sessions
- SQLite state with embedded forward migrations
- RBAC, CSRF protection and audit logging
- persistent jobs and durable events
- realtime WebSocket transport
- Module SDK v1, module registry and signed module-repository foundation
- CPU, RAM, network, GPU/PCI and block-device inventory
- Storage v1 management for non-system disks:
  - GPT/MBR inventory and unallocated capacity
  - partition create/delete/delete-all
  - ext4, XFS and FAT formatting
  - mount/unmount
  - filesystem labels and persistent display names
  - LVM inspection/deactivation for destructive operations
  - active-swap handling
  - SMART health, temperature, power-on time and SSD/NVMe lifetime where supported
- Web-driven Core update system:
  - architecture-specific update bundles
  - Core + Web UI + privileged helper in one verified bundle
  - SHA-256 and manifest verification
  - backup, restart and rollback
  - helper protocol/version compatibility
  - cached GitHub release discovery with fallback when the GitHub API is unavailable
- Debian 13 initial installer for amd64 and arm64

## Installation and updates

The Debian package is the bootstrap and emergency-recovery installation format.

After the initial installation, normal Home-AI-Core updates are installed from **System → Updates** in the Web UI. Normal updates do not require rebuilding or installing a new Debian package.

The initial installer can be built manually with the GitHub Actions workflow:

`Build initial installer`

or locally with:

```sh
sh ./scripts/build-deb.sh amd64
```

The Web update bundles are produced by:

```sh
sh ./scripts/build-update-bundle.sh amd64
sh ./scripts/build-update-bundle.sh arm64
```

## Runtime layout

The default Core listener is:

```text
127.0.0.1:8080
```

The public Core runs as the unprivileged `home-ai-core` account.

The privileged helper is installed at:

```text
/usr/libexec/home-ai-core/home-ai-core-updater
```

and communicates with Core through an authenticated Unix socket.

## Architecture principle

Core owns platform contracts, security, state, inventory, update orchestration and the minimal host operations required by the current platform.

Higher-level capabilities such as NAS/SMB/NFS, storage pools, snapshots, backup, containers, virtualization, NVR, smart-home services, AI runtimes and VPN functionality should remain modular rather than growing into one monolithic Core.

See `docs/architecture/`, `docs/decisions/` and `docs/update-system-v2.md` for the current contracts.
