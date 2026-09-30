# ADR-0024: Windows Credential Manager for background sync authentication

## Status

Accepted for implementation, 2026-09-30.

## Context

`0.1.76-dev` can keep scheduled push sync running only while a foreground process has access to `HOME_AI_PASSWORD`. Persisting that password in sync JSON, command-line arguments, a scheduled-task definition or logs would violate the client security boundary.

Windows Credential Manager provides per-user generic credential storage protected by Windows. Credential reads/writes are associated with the current user's logon credential set, so a future background launcher must run in that user's session rather than as LocalSystem.

## Decision

The Windows client adds a Credential Manager store keyed by normalized Home-AI server URL and username. The target name uses a SHA-256-derived application key; the credential metadata records the Home-AI account, while the secret blob contains only the password.

`HOME_AI_PASSWORD` remains the highest-priority source for explicit one-shot use. When it is absent on Windows, authentication falls back to Credential Manager. Linux/non-Windows builds keep the existing environment-only behavior.

The CLI exposes:

- `credentials save --server URL --username USER` — stores the current `HOME_AI_PASSWORD` in Windows Credential Manager;
- `credentials status --server URL --username USER` — reports whether a stored credential is available without printing it;
- `credentials delete --server URL --username USER` — removes the stored credential.

Queue and sync profile JSON files continue to contain no password or bearer token. Authentication failures never serialize the password into profile state.

## Security boundary

- passwords are never accepted as command-line arguments;
- passwords/tokens are never written to queue/sync JSON;
- Credential Manager persistence is local-machine, same-user scoped;
- secret buffers used for Win32 calls are cleared where practical;
- the client never prints the stored password;
- a future autostart/background agent must run under the same interactive user identity to access this credential.

## Consequences

Scheduled sync can authenticate after process restart without placing the password in Home-AI configuration files. A separate launcher/autostart slice is still required to start the agent after Windows logon. LocalSystem service mode is intentionally not the default design because it would not share the user's credential set.

## Deferred

- user-session autostart/background launcher;
- tray UI and credential prompts;
- device-bound/passwordless enrollment;
- credential rotation UX and multiple saved accounts UI.
