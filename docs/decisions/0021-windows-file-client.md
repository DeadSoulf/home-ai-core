# ADR-0021: Windows file-copy client foundation

## Status

Accepted.

## Context

Home-AI needs a first-party Windows client for copying selected files and folders into authorized private/shared NAS destinations. It is not a full system-image backup product.

The server already provides token-mode login and disk-backed resumable uploads with chunk and whole-file SHA-256 verification.

## Decision

The first Windows-client slice is a reusable Go client engine plus a Windows CLI.

Authentication uses POST /api/v1/auth/login with session_mode=token. The first CLI does not persist the Home-AI password or bearer token. The password is supplied through HOME_AI_PASSWORD.

Folder discovery uses GET /api/v1/files/folders and therefore exposes only server-authorized logical NAS folders.

Uploads reuse the existing resumable upload API. Before transfer the client computes the whole-file SHA-256. It resumes only a matching session, verifies historical chunk hashes against the selected local file, uploads sequential chunks with per-chunk SHA-256, retries transient failures, and verifies the final server size and SHA-256.

A mismatched unfinished upload is never silently replaced. Explicit stale replacement is exposed as --restart-stale.

The client is cross-built for Windows amd64 and published as a separate release asset with a SHA-256 file. It is not embedded in the Linux Core update bundle.

## Consequences

Benefits: a real Windows-to-Home-AI transfer path exists before GUI work; transfers resume after interruption; integrity is checked end-to-end; the same engine can later power GUI/background service workflows.

Current limitations: one explicit file per command, no persistent local queue, no recursive folder copy, no scheduler, no Credential Manager integration, and no GUI yet.

## Deferred

- Windows GUI and tray/service mode;
- persistent queue and recursive folder copy;
- scheduled/automatic sync and conflict policy;
- Windows Credential Manager/device enrollment;
- WireGuard remote-client integration.
