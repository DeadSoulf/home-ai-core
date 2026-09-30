# ADR-0027: Windows client update handoff and tray status

## Status

Accepted for implementation, 2026-09-30.

## Context

`0.1.79-dev` installs the Windows client at a stable per-user path and keeps the sync agent visible in the notification area. Updating that installed executable can still require the user to exit the running agent manually because Windows can keep the executable mapped while the process is alive.

The tray also needs enough status to answer two basic questions without opening JSON or logs: whether sync profiles are enabled and whether the most recent sync succeeded.

The solution must remain per-user, must not add a privileged service, and must not put Home-AI credentials into process arguments, registry startup values or update state.

## Decision

### Update handoff

`client install` and `agent install` use a bounded Windows handoff when the stable installed executable cannot be atomically replaced because it is in use.

The new client:

1. verifies whether the source and installed executable already have identical SHA-256 content;
2. attempts the existing temp-file + durable write + atomic replacement path;
3. only for Windows sharing/access conflicts, locates the Home-AI tray window by the product-owned window class and title;
4. posts the normal `WM_CLOSE` message so the existing agent cancels its scheduler, removes the tray icon and releases its process/file handles;
5. retries the atomic executable replacement for up to 15 seconds;
6. if autostart was enabled, starts the updated installed executable again with the managed sync-config path.

The managed HKCU Run command is parsed only in the exact canonical format produced by Home-AI:

`"<absolute-executable>" agent run --config "<absolute-config>"`

Arbitrary shell commands are not accepted or executed.

This handoff works when moving forward from `0.1.79-dev` because that release already owns the same tray window class and handles `WM_CLOSE` cleanly.

This is an install/update handoff for a new executable the user already has. It is not automatic release discovery or download.

### Tray status and notifications

The tray menu shows a read-only status line derived from persisted enabled sync profiles:

- number of enabled profiles;
- whether the latest enabled profile run is `OK` or `FAILED`;
- local time of that last attempt.

The tray tooltip is refreshed after actual sync cycles.

Balloon notifications are intentionally limited:

- any failed scheduled or manual cycle produces a failure notification;
- a manual **Sync now** produces a completion notification;
- successful background scheduled cycles update status silently.

The tray never displays stored passwords, bearer tokens or raw failure strings. Detailed errors remain in the rotating local agent log.

## Consequences

The common Windows update path no longer requires a manual **Exit → install → start** sequence. The replacement remains atomic and bounded, and the agent continues under the same logged-on Windows user and Credential Manager boundary.

A broken or non-canonical Home-AI Run value is rejected rather than interpreted as a shell command.

The notification area becomes a lightweight health surface without turning into a second configuration UI.

## Deferred

- release discovery and downloading a newer Windows client from Home-AI/GitHub;
- code-signature verification beyond the existing published SHA-256 asset;
- richer per-profile tray details and enable/disable controls;
- native installer/MSIX packaging;
- historical sync dashboard and desktop notifications policy;
- device-bound/passwordless enrollment.
