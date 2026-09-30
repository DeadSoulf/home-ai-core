# ADR-0028: Native Windows settings UI over the existing client backend

## Status

Accepted for implementation, 2026-09-30.

## Context

The Windows file client already has a stable per-user executable, Credential Manager integration, persistent sync profiles, a single-instance background agent and a native notification-area tray. The remaining setup path still requires PowerShell commands to save credentials, discover NAS folder IDs, create sync profiles and enable the agent.

That command-line flow is useful for diagnostics but is not an acceptable primary experience for a normal Windows client.

The settings experience must not fork a second sync implementation, must not place passwords or bearer tokens in local JSON, and must not create a second concurrent scheduler when the tray agent is already running.

## Decision

The existing `home-ai-windows-client.exe` receives a native Win32 settings window while preserving every existing CLI command.

On Windows, launching the executable without arguments opens the settings window. `settings` is also an explicit command for diagnostics and tray handoff.

The settings window provides:

- Home-AI server URL and username;
- a masked password field used only to validate/login and then save through Windows Credential Manager;
- discovery of writable Home-AI/NAS folders after authentication;
- a native local-folder picker;
- sync profile creation and editing;
- enable, disable and delete actions for existing profiles;
- sync interval and explicit conflict policy;
- background-agent/autostart controls;
- manual **Sync now** and current agent/autostart state.

The lightweight client settings file stores only the last server URL and username. The password remains in Windows Credential Manager. Sync profiles remain in the existing versioned profile file and continue to contain no password or bearer token.

Profile edits use the existing sync-profile backend and preserve the profile identity, enabled state, creation timestamp and previous run status/history fields.

If the tray agent is running, **Sync now** signals that existing process. The GUI does not start a second scheduler. A direct foreground sync is used only when no agent is running.

The tray gains a **Settings** action, and double-clicking the tray icon opens the settings window. Only one settings window is brought to the foreground for the current user session.

The implementation uses Win32 APIs from the existing Go executable and introduces no Electron, .NET or separate GUI runtime dependency.

## Consequences

A normal first-time Windows setup no longer requires the user to copy folder IDs or construct sync commands by hand.

CLI commands remain available as a recovery and diagnostic surface and continue to share the same backend/state files as the GUI.

The application remains a per-user client with no administrator or LocalSystem requirement.

## Deferred

- a conventional Windows installer / Start Menu integration and uninstall entry;
- automatic GitHub release discovery/download and verified self-update;
- richer transfer progress/history inside the GUI;
- localization and visual polish beyond the first native settings surface;
- device-bound/passwordless enrollment.
