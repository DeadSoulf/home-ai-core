# ADR-0025: Per-user Windows sync agent autostart

## Status

Accepted for implementation, 2026-09-30.

## Context

`0.1.77-dev` can authenticate scheduled sync after a process restart through Windows Credential Manager, but the user still has to start `sync watch` manually. The background launcher must use the same Windows user identity as the credential store, must not require administrator rights, and must not persist a password in its startup command.

Microsoft documents `HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\Run` as a per-user mechanism that runs a command whenever that user logs on. This matches the Home-AI credential boundary better than a LocalSystem service.

## Decision

The first background-agent slice uses the current user's `Run` registry key. `agent install` registers the current client executable with only:

`agent run --config <absolute-sync-profile-path>`

The Run value contains no password, bearer token, server credential or sync destination. Before installing autostart, the client requires at least one enabled sync profile and verifies that each distinct enabled server/user account has a stored Windows Credential Manager password.

`agent run`:

- acquires an operating-system file lock derived from the sync-config path so only one agent owns a profile set;
- hides the console window on Windows;
- appends diagnostics to a per-user local log and rotates one previous file when the log exceeds 4 MiB;
- reuses the existing `sync watch` scheduler and Credential Manager authentication;
- remains in the logged-on user's security context.

`agent status` reads the registered command and `agent remove` removes only the Home-AI Run value.

The Windows Run command is rejected when it exceeds the documented 260-character command-line limit. Executable and config paths must be absolute and may not contain quotes/control characters.

## Consequences

The Home-AI sync agent starts automatically after Windows user logon and can continue scheduled push sync without storing credentials in JSON, environment variables, or the Run key. It does not run before user logon and it is not a system service.

The registered executable path is explicit. Users must keep that executable at the registered path or run `agent install` again after moving/replacing it. A future installer can provide a stable per-user installation path.

## Deferred

- tray UI and foreground status/controls;
- automatic executable installation/update into a stable per-user location;
- IPC/health endpoint between tray and background agent;
- device-bound/passwordless enrollment;
- sync notifications and richer log/history UI.
