# ADR-0022: Persistent Windows copy queue and directory copying

## Status

Accepted, 2026-09-30.

## Context

The Windows file client copies one file using resumable server uploads. Users also need to copy a directory tree and continue a collection of transfers after a process or network interruption. The server's existing folder-scoped API and upload integrity checks remain the transfer contract.

## Decision

Directory copying starts with a deterministic local plan containing directories, including empty ones, and regular files. Each file records its absolute source, size, modification time and SHA-256. Symlinks and unsafe relative destinations are rejected before transfer; files are revalidated when executed. A changed source is a visible failure and requires a new plan.

An optional local JSON queue persists the plan and transfer states. One queue belongs to one normalized server URL and username. It stores no password or session token. Mutations hold an operating-system lock that is released when the process exits, and replace the queue file atomically. Reads and writes share a 64 MiB checkpoint limit; an oversized plan is rejected before replacing the saved queue. Interrupted running work becomes pending on reopen; completed work remains completed. Failed work requires an explicit retry and stops subsequent work until addressed.

The CLI exposes immediate `copy` and explicit `queue add`, `queue list`, `queue run` and `queue retry`. Every run authenticates again using the existing process-only password mechanism. The default queue lives in the user's configuration directory, with an explicit path option for separate queues.

Directory creation is idempotent only when the existing target is a directory. Existing files count as completed only after matching size and streamed SHA-256 verification. Content verification uses caller cancellation and an inactivity deadline, rather than the per-chunk total request timeout. This also resolves an interruption after the server committed a file but before the client saved completion. A different existing target is never overwritten. The server commits resumable files using an atomic no-replace operation, closing the former check-then-rename race.

## Alternatives considered

- Store credentials with queue entries: rejected; credential storage and device enrollment remain separate work.
- Treat every existing target as completed: rejected because it could hide different file contents.
- Replan directories automatically on resume: rejected because it changes the user's selected copy set after interruption.
- Rely only on the server upload list: insufficient to preserve a multi-file plan and completed items locally.

## Consequences

The existing resumable upload, scoped permissions, integrity checks and stale-upload replacement policy are reused without a new server API. Local plans and full SHA-256 verification add disk reads before transfer; resolving an existing server file requires reading it back. Completed queue entries are retained for inspection. The queue is sequential and stops at the first failure.

Scheduling, automatic synchronization, file replacement/conflict policy, retention of queue history, GUI/tray and credential management remain later milestones. Live SMB and physical Windows acceptance are tracked separately from automated tests.
