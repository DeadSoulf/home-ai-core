# ADR-0026: Stable per-user Windows installation and tray controls

## Status

Accepted for implementation, 2026-09-30.

## Context

The Windows sync agent can already start after user logon and authenticate through Windows Credential Manager, but the HKCU Run entry currently points to whichever executable path was used for `agent install`. Moving or deleting a downloaded release asset therefore breaks autostart. The background agent also has no visible user-session controls.

The solution must remain per-user, require no administrator rights, keep credentials out of command lines and JSON, and preserve the existing single-instance scheduler.

## Decision

The Windows client gains a stable per-user installation path:

`%LOCALAPPDATA%\HomeAI\bin\home-ai-windows-client.exe`

`client install` copies the currently running release executable to that path using a temporary file, fsync, atomic replacement and SHA-256 verification. Symlink sources and non-regular destination paths are rejected.

`agent install` now performs the same per-user client installation first and registers the HKCU Run command against the installed executable rather than the arbitrary download location. The sync-profile path remains explicit in the Run command; passwords and bearer tokens remain absent.

The background agent gains a native Win32 notification-area icon without adding a third-party GUI dependency. The tray menu exposes:

- **Sync now** — signals the existing scheduler loop and runs enabled profiles sequentially in the same process;
- **Open log** — opens the rotating per-user agent log;
- **Open sync profiles** — opens the sync-profile JSON with the user's associated application;
- **Exit** — cancels the agent context, removes the tray icon and releases the existing single-instance lock.

The scheduler is refactored to accept an optional in-process manual trigger. It never starts a second concurrent sync process.

## Update behavior

Replacing an installed executable is atomic when Windows permits the target file to be replaced. If the installed client is currently locked by a running agent, the user exits the agent from the tray and reruns `client install`. A future release may add a deferred self-update handoff, but this slice does not create a second updater or a privileged service.

## Consequences

Benefits:

- HKCU Run no longer depends on the Downloads path;
- release assets can be installed into a predictable current-user location;
- the logged-on user gets visible status presence and basic controls;
- manual **Sync now** reuses the scheduler and single-instance boundary;
- no administrator privileges or new credential storage are introduced.

Trade-offs:

- updating the executable can require exiting the running tray agent first;
- the tray is a lightweight control surface, not yet a full profile editor;
- no IPC endpoint exists for external GUI processes;
- install/update is still initiated explicitly by the user.

## Deferred

- deferred/self-handoff update while the installed executable is running;
- richer tray status, notifications and per-profile state;
- graphical profile editor;
- signed installer/MSIX packaging;
- device-bound/passwordless enrollment.
