# ADR-0010: Debian Initial Installation Package

- Status: Amended
- Original date: 2026-09-28
- Amended: 2026-09-29

## Context

Home-AI-Core needs a reproducible bootstrap format for clean Debian 13 machines without requiring Go, Node.js or a Git checkout on the target server.

The project now also has a Web-driven application updater, so package installation and normal application updates have different responsibilities.

## Decision

Use a native Debian package for **initial installation, emergency recovery and package-level transitions**.

The package installs:

- `/usr/bin/home-ai-core`
- the production Web UI
- `home-ai-core.service`
- `/usr/libexec/home-ai-core/home-ai-core-updater`
- `home-ai-core-updater.service`
- the system-user declaration
- bootstrap configuration
- required runtime tools such as filesystem/LVM/SMART utilities

Supported architectures are amd64 and arm64 on Debian 13.

## Normal updates

A normal Home-AI-Core release is not installed by building another Debian package.

After bootstrap, System → Updates installs a verified architecture-specific bundle containing:

- Core binary
- Web UI
- updater helper
- manifest/checksum metadata

The helper backs up the current application, performs the switch, restarts Core and supports rollback.

## Persistent state

Both initial package installation and later Web updates preserve:

- `/etc/home-ai-core/`
- `/var/lib/home-ai-core/`
- SQLite state and forward migrations

Package removal intentionally does not erase persistent state.

## Consequences

- clean Debian installation remains conventional;
- normal upgrades no longer depend on apt or a newly built `.deb`;
- privileged update/install logic remains outside the network-facing Core;
- package rebuilds are reserved for bootstrap/recovery or changes that truly require Debian-level installation changes.
