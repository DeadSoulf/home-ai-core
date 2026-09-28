# Home-AI-Core

Home-AI-Core is a modular private home-server platform built on Debian 13.

Its long-term goal is to unify personal storage, photos/video, backups, applications, video surveillance, smart-home services, local AI and multiple physical servers behind one secure control plane and Web UI.

## Current foundation

The restarted project currently includes:

- Go control-plane daemon
- Web UI with first-run setup/login
- SQLite state and migrations
- users, sessions, RBAC, CSRF protection and audit
- persistent jobs and durable events
- WebSocket realtime transport
- Module SDK v1 and persistent module registry
- read-only hardware discovery
- systemd service hardening
- Debian 13 package builds for amd64 and arm64

## Installation status

The project is entering its first physical-server validation phase.

Local Debian packages can be built with:

```sh
sh ./scripts/build-deb.sh amd64
```

or:

```sh
sh ./scripts/build-deb.sh arm64
```

See `packaging/debian/README.md` for installation and first-run instructions.

The default Core listener remains `127.0.0.1:8080`; remote first-run access uses an SSH tunnel until the secure remote-access phase is implemented.

## Architecture principle

The Core remains small.

Docker/containers, storage/NAS, virtualization, AI, NVR, backup, network/VPN and other product capabilities belong to independent modules rather than the Core itself.

See `docs/architecture/` and `docs/decisions/` for the current contracts and ADRs.
