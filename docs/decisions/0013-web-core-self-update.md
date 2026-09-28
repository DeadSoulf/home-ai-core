# ADR-0013: Web-driven Core self-update boundary

- Status: Accepted
- Date: 2026-09-28

## Context

Home-AI-Core must be maintainable from its Web UI. Requiring SSH, GitHub CLI and manual `apt install` for every Core update is not an acceptable normal operating path.

The public Core process is deliberately unprivileged, while Debian package installation requires root.

## Decision

### User flow

The Web UI exposes an Updates page with:

- installed Core version
- latest compatible development release
- release notes
- explicit check action
- explicit install action
- persistent update job identity
- automatic page reconnection after the Core restarts

### Release feed

Development builds are published from successful `main` CI runs as GitHub pre-releases.

Each release contains:

- amd64 Debian package
- arm64 Debian package
- `home-ai-core-update.json`
- exact package byte size
- SHA-256
- Debian package version

Core never accepts a package URL supplied by a Web client. It discovers the candidate from the fixed official release feed.

For the development channel, GitHub HTTPS plus release metadata is the current distribution trust anchor and SHA-256 provides package integrity between discovery, staging and privileged installation.

Detached Ed25519 signing of the Core update manifest remains required before the production update channel is considered complete.

### Job boundary

Installing an update creates a persistent `core.update.install` job.

The unprivileged Core:

1. discovers the newest version;
2. downloads the package into `/var/lib/home-ai-core/updates`;
3. enforces a maximum package size;
4. verifies exact size and SHA-256;
5. asks the privileged helper to install the already staged package.

### Privileged helper

`home-ai-core-update-helper` runs as a separate root systemd service and listens only on a Unix socket writable by the `home-ai-core` group.

The helper:

- verifies the peer UID is the Home-AI-Core service account;
- accepts only the typed `install-core-update` operation;
- accepts packages only from the fixed update staging directory;
- rejects symlinks and group/world-writable package files;
- recomputes SHA-256;
- verifies Debian package name is exactly `home-ai-core`;
- verifies architecture and expected Debian version;
- refuses downgrades;
- invokes `apt-get` with fixed arguments and keeps existing conffiles.

No shell command, executable path or arbitrary apt package name is accepted from the Web/API request.

### Self-restart recovery

The package post-install script restarts Home-AI-Core.

If the update job was running when the old process exits, the existing job recovery mechanism requeues it. The new Core process sees that the target version is already installed and completes the job idempotently.

The helper handles one privileged request per process and exits. systemd restarts it, ensuring an upgraded helper binary is loaded after a Core self-update.

## Consequences

### Positive

- normal Core updates no longer require SSH after the bootstrap updater release;
- the network-facing Core remains unprivileged;
- privileged behavior is narrowly typed and auditable;
- interrupted self-restarts are recoverable;
- current network/listener configuration remains a Debian conffile and is preserved during update.

### Remaining work

- detached Ed25519 signing for production Core release manifests;
- automatic rollback to the previous package on failed post-update health checks;
- release-channel selection and production/stable policy;
- physical-node acceptance of the full Web update cycle.
