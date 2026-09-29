# ADR-0013: Web-driven Core update boundary

- Status: Accepted, revised for Update System v2
- Original date: 2026-09-28
- Revised: 2026-09-29

## Context

Home-AI-Core must be maintainable from its Web UI without SSH for routine updates.

The network-facing Core is unprivileged, while replacing installed binaries and accessing protected host resources requires root. Normal updates must therefore preserve privilege separation without depending on a newly built Debian package.

## Decision

### Update artifact

GitHub publishes one update bundle per supported architecture:

```text
home-ai-core-update_<version>_<arch>.tar.gz
home-ai-core-update_<version>_<arch>.tar.gz.sha256
```

The bundle contains:

- `manifest.json`
- `bin/home-ai-core`
- `helper/home-ai-core-updater`
- `web/...`

The manifest records product, version, architecture, helper protocol/version and SHA-256/size for every file.

### Discovery

Core normally discovers releases from the official GitHub release feed.

Successful checks are cached and persisted. If the GitHub API is unavailable or rate-limited, discovery can fall back to the published `VERSION` file plus verification that the expected release asset exists.

The browser cannot supply an arbitrary update URL.

### Download and verification

The unprivileged Core downloads the exact selected archive into its update state directory and verifies:

- archive checksum
- size limits
- manifest product/version/architecture
- safe relative paths
- per-file size and SHA-256
- required Core/Web/helper files

### Privileged installation

The root helper:

1. verifies the prepared bundle again;
2. backs up the installed Core, Web UI and helper;
3. stops Core;
4. atomically switches Core/Web files;
5. starts Core and verifies service health;
6. replaces the helper and restarts its systemd service when needed.

The helper protocol is versioned so newer Core code can detect an incompatible installed helper before attempting an operation.

### Rollback

The previous application backup is exposed through the update API/Web UI. Rollback restores Core, Web and the helper and restarts services.

### Debian package boundary

The `.deb` is not the normal update transport. It remains the initial/bootstrap and emergency recovery mechanism, and is used when a package-level transition cannot safely be achieved by the running bundle updater.

## Security properties

- Core remains unprivileged.
- Helper access is restricted to the `home-ai-core` service account over AF_UNIX.
- No arbitrary command or executable path is accepted from Web input.
- Bundle contents are bounded and verified before privileged installation.
- Only one install/rollback operation can run at a time.
- Update mutations require authenticated permission and CSRF protection.

## Remaining production work

Detached signing and stable-channel release policy remain separate production-hardening tasks beyond the current development-channel SHA-256/GitHub trust model.
