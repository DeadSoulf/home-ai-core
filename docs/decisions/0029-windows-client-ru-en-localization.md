# ADR-0029: Russian and English localization for the Windows client

## Status

Accepted for implementation, 2026-09-30.

## Context

The native Windows settings client introduced in 0.1.81-dev is intended to replace PowerShell as the normal setup surface. The first GUI was English-only, while the Windows client needs a user-selectable Russian and English interface without maintaining separate executables or duplicating sync logic.

Passwords and tokens must remain outside localization/state files, and the tray process must follow the same language selection as the settings window.

## Decision

The Windows client uses one executable and one shared translation table for `ru` and `en`.

On first launch, the client reads the current Windows user locale. Russian Windows locales default to Russian; all other locales default to English. Once the user selects a language in the settings window, the explicit `ru` or `en` value is saved in the existing non-secret client settings JSON and overrides locale detection on later runs.

The settings window applies the language immediately. Labels, buttons, conflict-policy labels, status messages, confirmations and client-side validation are localized. The selected language is preserved when connection settings are saved.

The background tray reads the same saved language preference. Tray actions, sync summaries, tooltips and sync notifications are localized. A settings-window language change sends a refresh message to a running Home-AI tray so the new preference is picked up without restarting the agent.

Home-AI server-provided names and low-level technical error details are not machine-translated. They remain verbatim, with localized surrounding UI/status text. The diagnostic CLI remains English in this milestone.

## Consequences

There is still one Windows binary and one sync/agent implementation. Language selection does not alter credentials, sync profiles or transfer behavior.

The settings JSON may contain the non-secret `language` field alongside server URL and username. Passwords remain exclusively in Windows Credential Manager.

## Deferred

- additional UI languages;
- full CLI localization;
- localization of server-originated error payloads;
- installer language selection.
