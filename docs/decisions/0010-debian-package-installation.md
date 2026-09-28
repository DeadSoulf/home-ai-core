# ADR-0010: Debian Installation Package

- Status: Accepted
- Date: 2026-09-28

## Context

After Web UI Phase 8A, Home-AI-Core is ready for its first installation on physical Debian 13 hardware.

The project needs a reproducible installation format that can later evolve into signed repositories and managed updates.

Installing directly from a Git checkout would make production nodes depend on Go, Node.js, npm and source-tree layout.

## Decision

Home-AI-Core is installed as a native Debian package.

The package contains:

- the prebuilt Go Core binary
- the prebuilt Web UI static assets
- the systemd service
- the system-user declaration
- bootstrap environment configuration

Build dependencies do not become runtime dependencies.

### Supported systems

Initial package targets:

- Debian 13 amd64
- Debian 13 arm64

### Runtime identity

The public Core continues to run as the unprivileged `home-ai-core` system account.

The package does not weaken ADR-0003 privilege separation and does not introduce the future privileged helper.

### Device visibility

Core needs read-only visibility of host device inventory for capability discovery.

The systemd unit therefore keeps device nodes visible but uses `DevicePolicy=closed` and an empty capability set. Core can inspect the presence of devices such as `/dev/kvm` but cannot open arbitrary devices.

### Configuration and state

Package upgrades preserve:

- `/etc/home-ai-core/home-ai-core.env`
- `/var/lib/home-ai-core/`

Database schema migrations remain owned by Core startup.

Package removal does not automatically erase persistent state.

### Network exposure

First physical installation remains loopback-only.

Remote first-owner setup uses an SSH tunnel.

The installer does not change firewall rules, expose port 8080 externally or install a reverse proxy.

### Future distribution

Phase 8.5 creates local `.deb` artifacts.

Signed APT repositories, release signatures, update channels, rollback snapshots and automatic upgrades remain Phase 9/Update Manager work.

## Consequences

### Positive

- physical servers do not require Go or Node.js
- installation matches normal Debian administration
- systemd ownership/configuration is explicit
- upgrades preserve state and config
- later signed APT distribution can use the same artifact format

### Negative

- package signing is not implemented yet
- rollback is not yet automatic
- release artifact publishing is still manual
