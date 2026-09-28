# Update System v2 Plan

## Core rule

The Debian package is only for the initial installation and emergency recovery.

Normal Home-AI-Core updates from the Web UI do **not** build or install a new .deb package.
They use a small application update bundle containing only the Core binary, Web UI and metadata.

There is no unattended auto-update. Installation always starts from an explicit Web UI action.

## Update artifact

For each supported architecture GitHub publishes:

```
home-ai-core-update_<version>_<arch>.tar.gz
home-ai-core-update_<version>_<arch>.tar.gz.sha256
```

Bundle layout:

```
manifest.json
bin/
  home-ai-core
web/
  index.html
  assets/...
```

The bundle is produced by:

```bash
sh ./scripts/build-update-bundle.sh amd64
sh ./scripts/build-update-bundle.sh arm64
```

Output directory:

```
build/updates/
```

## Manifest

`manifest.json` contains:

- schema version
- product name
- Home-AI-Core version
- architecture
- every update file
- SHA-256 for every file
- exact file size

The archive itself also has a sibling `.sha256` checksum file.

## Runtime directories

Initial .deb installation creates:

```
/var/lib/home-ai-core/update/
```

This directory is owned by the unprivileged `home-ai-core` service account and is used only for downloading and staging an update.

The live installation remains:

```
/usr/bin/home-ai-core
/usr/share/home-ai-core/web/
```

## Runtime update flow

### 1. Check

Web UI button:

**Check for updates**

Core queries GitHub Releases and finds the newest compatible update bundle for the running architecture.

The UI displays:

- current version
- available version
- release date
- archive size
- release notes

No files are downloaded during the check.

### 2. Download

Web UI button:

**Download update**

Core downloads the exact selected bundle into:

```
/var/lib/home-ai-core/update/
```

Core verifies:

1. archive SHA-256
2. archive size
3. `manifest.json`
4. product name
5. architecture
6. version
7. every contained file SHA-256
8. every contained file size
9. safe relative paths

After verification the updater state becomes `ready`.

### 3. Install

Web UI button:

**Install update**

A minimal privileged installer performs only the local filesystem switch.

It does not access GitHub and does not choose a version.

Installation sequence:

1. verify the already-staged bundle again
2. stop `home-ai-core.service`
3. create a backup of the current binary and Web UI
4. install the new binary
5. replace the Web UI atomically
6. start `home-ai-core.service`
7. verify `/health`
8. verify the running version

### 4. Rollback

If the new Core does not start or reports the wrong version:

1. stop the failed service
2. restore the previous binary and Web UI
3. start the previous version
4. record update state as `failed`

The previous working version must remain recoverable until the new version has passed verification.

## Updater states

The updater is independent from the generic Jobs subsystem.

States:

```
idle
checking
available
downloading
ready
installing
restarting
succeeded
failed
```

Go definitions live under:

```
internal/updater/
```

## API target

### GET /api/v1/update

Returns current updater state and discovered release information.

### POST /api/v1/update/check

Explicitly checks GitHub.

### POST /api/v1/update/download

Body:

```json
{"version":"0.2.0"}
```

Downloads and verifies exactly that version.

### POST /api/v1/update/install

Body:

```json
{"version":"0.2.0"}
```

Starts installation of an already verified staged update.

### GET /api/v1/update/state

Returns phase, progress, message and any error.

## Security rules

- Network-facing Core always runs as `home-ai-core`, never root.
- The browser never supplies a local path.
- The browser never supplies a download URL.
- GitHub URL is derived by Core from release metadata.
- SHA-256 verification is mandatory.
- Manifest validation is mandatory.
- Unsafe archive paths are rejected.
- Architecture mismatch is rejected.
- Downgrades are rejected by default.
- Only one updater operation may exist at a time.
- Install requires authenticated owner/admin permission and CSRF validation.
- The privileged installer has no network responsibilities.

## Initial .deb responsibilities

The .deb remains responsible for the first machine installation:

- create `home-ai-core` system user
- install systemd service
- install initial Core binary
- install initial Web UI
- create state/config/update directories
- later: install the minimal privileged updater helper

The .deb is not part of the normal Web update path.

## Implementation phases

### Phase 1 — update bundle format
Status: implemented foundation.

- `scripts/build-update-bundle.sh`
- deterministic tar.gz
- manifest with per-file checksums
- archive SHA-256
- `internal/updater` manifest/state types
- update staging directory

### Phase 2 — bundle release publishing

Create a GitHub workflow that only builds and publishes update bundles.
It has no runtime installation logic.

### Phase 3 — discovery API

Implement GitHub release discovery and version comparison.

### Phase 4 — Web check UI

Add System -> Updates with an explicit check button.

### Phase 5 — secure bundle download

Download, unpack into a temporary directory and verify all manifest entries.

### Phase 6 — privileged local installer

Implement backup, atomic replacement, restart and rollback.

### Phase 7 — Web install flow

Wire download/install/progress/reconnect into the Web UI.

### Phase 8 — failure testing

Test:

- GitHub unavailable
- no update
- wrong architecture
- bad archive checksum
- invalid manifest
- path traversal attempt
- partial download
- corrupt file inside archive
- service fails after replacement
- wrong running version
- rollback
- already-installed version
- attempted downgrade

## Acceptance criteria

The updater is complete when:

1. Initial installation can still be done with one .deb.
2. Future Web updates do not require another .deb.
3. Web UI discovers a newer GitHub bundle.
4. User explicitly starts download/install.
5. Exact selected version is downloaded.
6. Archive and all files are verified.
7. Existing binary/Web UI are backed up.
8. Core and Web UI are replaced.
9. Core restarts into the selected version.
10. Web UI reconnects and confirms success.
11. Failed upgrade automatically restores the previous version.
12. SSH is not required for a normal update.
