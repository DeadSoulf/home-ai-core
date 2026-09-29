# Update System v2

## Core rule

The Debian package is the bootstrap and emergency-recovery installation format.

Normal Home-AI-Core updates are performed from the Web UI with an application bundle. They do not build or install a new Debian package.

There is no unattended auto-update. Installation starts only from an explicit user action.

## Update artifact

For each architecture GitHub publishes:

```text
home-ai-core-update_<version>_<arch>.tar.gz
home-ai-core-update_<version>_<arch>.tar.gz.sha256
```

Bundle layout:

```text
manifest.json
bin/home-ai-core
helper/home-ai-core-updater
web/index.html
web/assets/...
```

The manifest contains:

- schema version
- product
- Home-AI-Core version
- architecture
- updater-helper protocol/version
- exact size and SHA-256 for every bundled file

## Runtime flow

### Check

`GET /api/v1/update` returns cached release information when it is still fresh.

A manual Web check calls the same endpoint with `?fresh=1`.

Release discovery normally uses the GitHub Releases API. Successful results are cached in memory and persisted. If the GitHub API is unavailable or rate-limited, Core can fall back to the published `VERSION` file and the expected release asset.

### Download

`POST /api/v1/update/download` downloads the exact architecture/version bundle selected by Core.

Core verifies:

1. archive SHA-256
2. bounded archive size
3. safe extraction paths
4. manifest schema/product/version/architecture
5. helper protocol metadata
6. exact file sizes
7. SHA-256 of every extracted file
8. absence of unexpected files

A verified bundle enters `ready`.

### Install

`POST /api/v1/update/install` asks the privileged helper to install the already verified staged bundle.

The helper:

1. verifies the prepared bundle again
2. backs up Core, Web UI and helper
3. stages replacements
4. stops Core
5. switches Core and Web UI
6. starts Core and verifies systemd health
7. switches the helper binary when present
8. restarts the helper service to activate its new version

### Rollback

`POST /api/v1/update/rollback` restores the previous Core/Web/helper backup when one is available.

The Web UI exposes the rollback target version and reconnects after service restart.

## Updater states

```text
idle
checking
available
downloading
ready
installing
rolling_back
restarting
succeeded
failed
```

`available` and `checking` are informational states, not fake 0% progress operations.

## Privilege boundary

- Core runs as `home-ai-core`.
- The helper runs as root.
- Communication uses `/run/home-ai-core-updater.sock`.
- The helper verifies the peer UID.
- Requests are typed and protocol-versioned.
- The browser never supplies an executable path, local staging path or arbitrary shell command.
- Storage and update operations share the privileged helper protocol but have separate allowlisted operations.

## Initial installer responsibilities

The initial Debian installer provides:

- service user/group
- Core binary
- Web UI
- root helper
- systemd units
- config/state directories
- required storage/SMART/LVM utilities

After this bootstrap, normal application updates use Web bundles.

## Current acceptance

Implemented:

- architecture-specific bundles
- Core/Web/helper packaging
- archive and per-file verification
- Web check/download/install
- helper compatibility discovery
- backup and rollback
- Core/helper restart handling
- persisted successful update-check cache
- GitHub API fallback discovery
- Web notification for a genuinely newer release

Remaining production hardening includes signed stable-channel manifests and formal long-duration failure testing.
