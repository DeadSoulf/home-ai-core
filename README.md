# Home-AI-Core

Home-AI-Core is the trusted control-plane foundation of **Home-AI** — an autonomous local platform for smart-home automation, cameras/NVR, personal file storage, local AI, voice interaction and future multi-node resource sharing.

The long-term product is designed to keep essential home functions local and operational without mandatory cloud services or subscriptions.

The current project is intentionally building the foundation first: identity, security, jobs, events, modules, system/storage control, updates and the Web UI that future Home-AI domains will use.

## Product direction

Home-AI is planned around these primary domains:

- native Smart Home runtime and automation engine;
- local Cameras / NVR with recording, retention and AI vision;
- File Storage / NAS with private and shared user spaces;
- local AI Agent with permission- and approval-controlled tools;
- voice terminals;
- secure remote access through an owner-controlled tunnel;
- Windows file-upload client and later Android client;
- multi-node Home-AI cluster with resource-aware workload placement.

Home Assistant is not a required dependency of the target smart-home architecture.

Generic container/virtualization management may exist later as optional infrastructure, but it is not the primary product mission.

See:

- `docs/PRODUCT_VISION.md` — canonical product goal and requirements;
- `docs/PROJECT_MAP.md` — current roadmap/status;
- `docs/CURRENT_STATE_AUDIT.md` — what existing work is kept, repurposed or deprioritized.

## Current platform

The repository currently includes:

- Go control-plane daemon and React/TypeScript Web UI;
- first-run owner bootstrap and authenticated sessions;
- SQLite state with embedded forward migrations;
- RBAC, CSRF protection and audit logging;
- persistent jobs and durable events;
- realtime WebSocket transport;
- Module SDK v1, module registry and signed module-repository foundation;
- CPU, RAM, network, GPU/PCI and block-device inventory;
- Storage v1 management for non-system disks:
  - GPT/MBR inventory and unallocated capacity;
  - partition create/delete/delete-all;
  - ext4, XFS and FAT formatting;
  - mount/unmount;
  - filesystem labels and persistent display names;
  - LVM inspection/deactivation for destructive operations;
  - active-swap handling;
  - SMART health, temperature, power-on time and SSD/NVMe lifetime where supported;
- Web-driven Core update system:
  - architecture-specific update bundles;
  - Core + Web UI + privileged helper in one verified bundle;
  - SHA-256 and manifest verification;
  - detached Ed25519 verification for signed Core releases; non-development releases fail closed without a signature;
  - backup, restart and rollback;
  - helper protocol/version compatibility;
  - cached GitHub release discovery with fallback;
- Debian 13 initial installer for amd64 and arm64.
- Windows file-copy client with resumable uploads, recursive directory copying and a persistent local transfer queue; see [usage](docs/windows-file-client.md).

## Installation and updates

The Debian package is the bootstrap and emergency-recovery installation format.

After initial installation, normal Home-AI-Core updates are installed from **System → Updates** in the Web UI. Normal updates do not require rebuilding or installing a new Debian package.

The initial installer can be built with GitHub Actions or locally:

```sh
sh ./scripts/build-deb.sh amd64
```

Update bundles:

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

Core owns shared platform contracts:

- identity/security;
- permissions/policy;
- state;
- jobs/events;
- audit;
- module registry;
- update orchestration;
- node/system capability discovery.

Product domains such as NAS, Smart Home, NVR, AI, Voice, WireGuard and Cluster should be modular and reuse these shared contracts rather than bypassing them.


## License

Home-AI-Core is licensed under the Apache License 2.0. See [LICENSE](LICENSE).
