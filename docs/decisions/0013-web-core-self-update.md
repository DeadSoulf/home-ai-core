# ADR-0013: Web-driven Core update boundary

- Status: Accepted, revised for Update System v2
- Original date: 2026-09-28
- Revised: 2026-10-01

## Context

Home-AI-Core must be maintainable from its Web UI without SSH for routine updates.

The network-facing Core is unprivileged, while replacing installed binaries and accessing protected host resources requires root. Normal updates must therefore preserve privilege separation without depending on a newly built Debian package.

## Decision

### Update artifact

GitHub publishes one update bundle per supported architecture:

```text
home-ai-core-update_<version>_<arch>.tar.gz
home-ai-core-update_<version>_<arch>.tar.gz.sha256
home-ai-core-update_<version>_<arch>.tar.gz.sha256.sig
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

- detached Ed25519 signature over the exact checksum-file bytes when a signature is present;
- non-development releases require that detached signature and are ignored/rejected if it is absent;
- the trusted public key comes from the local `/etc/home-ai-core/update-trusted.pub` trust root;
- archive checksum;
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
- A release-site attacker cannot replace a stable bundle and checksum without also producing a valid Ed25519 signature from the separately held project signing key.
- Development releases remain allowed to operate unsigned until the dedicated signing key is provisioned; this exception does not apply to stable/RC releases.
- Only one install/rollback operation can run at a time.
- Update mutations require authenticated permission and CSRF protection.

## Remaining production work

The detached-signature protocol and stable fail-closed policy are implemented. Remaining operational work is provisioning the dedicated private key into the `HOME_AI_UPDATE_SIGNING_KEY` Actions secret, installing the matching public key on managed nodes, defining key rotation/revocation, and running long-duration failure tests.
