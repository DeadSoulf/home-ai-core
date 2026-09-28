# Update System v2 Plan

## Goal

Home-AI-Core must update from the Web UI with one clear flow:

1. Check GitHub for a newer published package.
2. Show current and available versions.
3. User presses **Install update**.
4. Core downloads the exact package selected during the check.
5. Package integrity and compatibility are verified.
6. A privileged installer installs the package.
7. Home-AI-Core restarts.
8. Web UI reconnects and confirms the new running version.

No automatic installation. No background auto-update scheduler. No update jobs mixed into the general job history.

## Separation of responsibilities

### 1. Release publishing
GitHub-side responsibility only.

A release contains:
- one amd64 Debian package,
- one arm64 Debian package,
- one update manifest,
- SHA-256 and size metadata.

Publishing a release is independent from the runtime updater. Runtime code must not depend on GitHub Actions internals.

### 2. Update discovery
Unprivileged Home-AI-Core responsibility.

Endpoint:
- `GET /api/v1/update`

It:
- reads the current Core version,
- queries GitHub Releases,
- selects the newest compatible published release,
- reads its manifest,
- returns current version, available version, notes, architecture, package size and availability.

It never downloads or installs anything.

### 3. Package download
Unprivileged Home-AI-Core responsibility.

Endpoint:
- `POST /api/v1/update/download`

Input:
- exact version selected by the user.

It:
- revalidates that release,
- downloads the package to `/var/lib/home-ai-core/update/`,
- verifies expected size,
- verifies SHA-256,
- verifies package name, architecture and Debian version,
- records a simple updater state file.

UI states:
- idle
- checking
- available
- downloading
- ready
- installing
- restarting
- succeeded
- failed

This updater state is separate from the generic Jobs module.

### 4. Privileged installation
A small root-only installer is used only for the final install operation.

It must:
- accept only a package from the dedicated update staging directory,
- accept only package name `home-ai-core`,
- reject architecture mismatch,
- reject downgrade,
- run `apt-get install <exact staged file>`,
- write an installation result/status file.

It must not:
- access GitHub,
- select versions,
- download files,
- expose a network port,
- contain release logic.

### 5. Restart handling
The package post-install script restarts only `home-ai-core.service`.

The installer itself must not be restarted by the package being installed.

The Web UI expects temporary API loss and polls:
- `GET /health`
- then `GET /api/v1/system`

Success means the running Core version equals the selected version.

## Web UI

Location:
- System -> Updates

Display:
- Current version
- Available version
- Published date
- Package size
- Release notes
- Last check time
- Status/progress
- Error text when applicable

Buttons:
- **Check for updates**
- **Download**
- **Install**

For the normal path, after download succeeds the UI may offer one combined primary action:
- **Download and install**

Installation always requires an explicit user click.

## API proposal

### GET /api/v1/update
Returns discovery/status information.

### POST /api/v1/update/check
Optional explicit check endpoint if we do not want GET to perform an external request.

### POST /api/v1/update/download
Body:
```json
{"version":"0.2.0"}
```

### POST /api/v1/update/install
Body:
```json
{"version":"0.2.0"}
```

### GET /api/v1/update/state
Returns the current updater state and progress.

## Security rules

- Core continues to run as the unprivileged `home-ai-core` user.
- Only the minimal installer runs as root.
- CSRF protection is required for download/install actions.
- Only owner/admin permission may install.
- Package path is never accepted directly from the browser.
- Download URL is derived from trusted GitHub release metadata, not supplied by the browser.
- SHA-256 verification is mandatory.
- Package metadata validation is mandatory.
- Downgrades are rejected by default.
- Only one update operation may run at a time.

## Implementation order

### Phase 1 — clean package build
Ensure `scripts/build-deb.sh amd64` and `arm64` build without any updater code.

### Phase 2 — deterministic release format
Create a simple release build/publish workflow only after local package build is clean.
This workflow publishes packages; it does not participate in installation.

### Phase 3 — discovery API
Implement GitHub release check and manifest parsing only.
No install code yet.

### Phase 4 — Web check UI
Implement current/available version display and **Check for updates**.

### Phase 5 — staged download
Implement package download, SHA-256 verification and updater state.

### Phase 6 — privileged installer
Add a new minimal installer with a narrow protocol and strict package validation.

### Phase 7 — Web install flow
Connect the install button, restart detection and success/failure display.

### Phase 8 — failure tests
Test:
- GitHub unavailable
- no update
- wrong architecture
- bad SHA-256
- partial download
- invalid .deb
- apt failure
- Core restart during install
- already-installed version
- attempted downgrade

## Acceptance criteria

The updater is finished only when all of these work:

1. A newer GitHub release is detected from Web UI.
2. No shell/SSH command is required.
3. The exact selected package is downloaded.
4. Integrity is verified before root installation.
5. Clicking install upgrades Home-AI-Core.
6. Web UI survives the restart and reports success.
7. Failed installation leaves the previous installation recoverable.
8. Update failures show a clear error in the update page.
9. No general Jobs polling is required for updater state.
10. There is no automatic unattended installation.
