# ADR-0023: Scheduled Windows push sync and conflict policy

## Status

Accepted for implementation, 2026-09-30.

## Context

The Windows client already provides resumable uploads, recursive copy and a persistent copy queue. The next NAS milestone requires repeated automatic copying without turning the client into an unsafe bidirectional mirror.

Automatic replacement is especially sensitive: the server already treats an existing different destination as a conflict, and the recycle bin provides a recoverable alternative to destructive overwrite. Authentication secrets must remain outside the persisted client state until a dedicated Windows credential-store design exists.

## Decision

The first automatic synchronization mode is one-way **Windows → Home-AI push sync**.

A local versioned JSON profile stores only non-secret configuration:

- normalized Home-AI server URL and username;
- authorized logical folder ID;
- absolute local source;
- relative server destination;
- interval;
- enabled state;
- explicit conflict policy;
- last attempt/success/error metadata.

Passwords and bearer tokens are never stored in the profile. `HOME_AI_PASSWORD` remains process-only. `sync run` executes one selected profile immediately or all enabled profiles that are due. `sync watch` keeps a foreground process alive, reloads profile configuration periodically and executes profiles according to their persisted intervals.

Each run creates a fresh deterministic local plan. This is intentionally different from the persistent copy queue: synchronization must discover files added or changed after the previous run. Existing identical remote items are verified and skipped.

Conflict handling is explicit:

- `stop` — stop the profile on the first different existing destination;
- `skip` — leave the remote item unchanged and continue;
- `replace-to-trash` — move the exact conflicting destination to the Home-AI recycle bin, then copy the local item.

`replace-to-trash` never automatically removes a conflicting ancestor path. This prevents a nested source from discarding unrelated remote data.

The first sync mode is additive rather than mirror-style: a file deleted locally is **not** deleted remotely. Remote deletion propagation is deferred until it has a separate retention and recovery contract.

Profile writes are atomic and protected by an OS file lock. The profile file has a bounded size and rejects symlink/non-regular-file state paths.

## Consequences

Benefits:

- repeated folder sync works without changing the server data API;
- new local files are discovered on each run;
- existing content remains protected by explicit policy;
- replacement is recoverable through the existing recycle bin;
- credentials are not persisted;
- the same profile format can later power a tray/service UI.

Trade-offs:

- `sync watch` must remain running and requires the password in that process environment;
- there is no reboot-persistent credential integration yet;
- full bidirectional sync and deletion propagation are not implemented;
- `replace-to-trash` can grow recycle-bin usage until retention/cleanup policy is added.

## Deferred

- Windows Credential Manager/device enrollment;
- Windows service/tray integration and startup registration;
- bidirectional conflict resolution;
- local-delete propagation and mirror semantics;
- bandwidth windows and advanced schedules;
- sync history retention/pruning UI.
