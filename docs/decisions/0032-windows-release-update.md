# ADR-0032: Automatic Windows client release discovery and verified update handoff

## Status

Accepted for implementation, 2026-10-01.

## Context

The Windows client already has a stable per-user install path and a bounded handoff that can stop and restart the Home-AI tray agent while replacing the installed executable. Until now, the user still had to open a GitHub release, download the matching `.exe` and `.sha256` assets manually, then launch the downloaded executable with `client install`.

The next F2 slice needs to remove that manual discovery/download step without weakening the existing per-user and credential boundaries.

## Decision

The Windows client gains `client update`.

The update flow:

1. queries the public Home-AI GitHub releases API;
2. skips drafts and selects the newest release that contains an exact amd64 Windows asset pair:
   - `home-ai-windows-client_<version>_amd64.exe`;
   - `home-ai-windows-client_<version>_amd64.exe.sha256`;
3. rejects non-HTTPS release/download URLs outside loopback-only test servers;
4. bounds release metadata, checksum and executable response sizes;
5. parses the published `sha256sum` file and requires it to name the exact executable asset;
6. downloads the executable into the per-user Home-AI update cache;
7. verifies SHA-256 before activating the cached file and verifies the cached file again afterward;
8. launches the verified downloaded executable with `client install`;
9. exits the old process so the verified child can replace the stable installed executable;
10. if the stable executable is still locked briefly by the parent process, the existing bounded handoff retries for up to 15 seconds;
11. if the tray agent also owns the installed executable, the existing product-owned `WM_CLOSE` handoff stops it and restarts it after replacement when managed autostart is enabled.

The update path does not pass Home-AI credentials, passwords or bearer tokens to the child process. Release discovery uses only public GitHub release metadata and release assets.

The installed executable remains the stable per-user file:

`%LOCALAPPDATA%\\HomeAI\\bin\\home-ai-windows-client.exe`

Verified release downloads are cached under the current user's Home-AI cache directory so the child process is not dependent on a browser download location.

## Consequences

A user can update an installed Windows client with one command without manually visiting GitHub or matching checksum assets.

The published SHA-256 release asset remains the integrity boundary for this slice. The download is not treated as trusted until verification succeeds.

The handoff remains unprivileged and scoped to the current Windows user.

## Deferred

- Authenticode/code-signing verification in addition to SHA-256;
- update controls inside the native settings window and tray menu;
- background update checks or automatic unattended installation;
- release channels beyond the repository's current release stream;
- native installer/MSIX packaging and Windows uninstall registration.
