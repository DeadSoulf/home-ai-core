# ADR-0019: Resumable uploads with checksums

## Status

Accepted.

## Context

The first NAS file-data slice supports direct HTTP uploads with an atomic temp-file rename, but a single interrupted request requires restarting the entire upload. That is not suitable for large household files or a future Windows copy/sync client.

Upload state must also survive Core restarts without moving file payloads into SQLite.

## Decision

Home-AI uses disk-backed resumable upload sessions inside the logical folder boundary.

Each upload session has:

- a stable random upload ID;
- a target relative path;
- expected total size;
- received-byte offset;
- optional expected whole-file SHA-256;
- optional client fingerprint;
- per-chunk offset, size and SHA-256 metadata;
- created/updated timestamps.

The partial payload and JSON metadata are stored as hidden Home-AI upload artifacts under the logical folder root. They are excluded from normal file listings and cannot be addressed through the user-visible path resolver.

Chunks are accepted only at the exact current offset. Each chunk is limited to 8 MiB and may include an expected SHA-256. A failed checksum or oversized chunk does not advance the session.

Completion requires the received size to equal the declared total size. Core calculates SHA-256 for the complete partial file and compares it with the optional expected whole-file checksum. Only after successful verification is the partial file atomically renamed to the requested target.

Upload sessions are intentionally not stored in SQLite. The filesystem is the authoritative data plane for in-progress file transfers.

## API shape

The file API exposes operations to:

- create or discover an upload session;
- read upload status/offset;
- append the next chunk;
- finalize the upload;
- cancel the upload.

All operations are still scoped to a specific `file_folder` and inherit the existing `files.write` authorization, CSRF and audit boundaries.

## Web behavior

The Web file manager uses chunked upload with progress reporting.

If a matching unfinished upload exists for the same file fingerprint, the Web client resumes from the server-reported offset. If the unfinished session does not match the selected file, the user is asked before cancelling and restarting it.

The completed response includes the server-computed SHA-256.

## Consequences

Benefits:

- uploads survive interrupted HTTP requests and Core restarts;
- large files no longer require restarting from byte zero after interruption;
- chunk-level checksums detect transport/client corruption early;
- whole-file SHA-256 provides final integrity verification;
- the same contract can be reused by the future Windows copy/sync client.

Trade-offs:

- abandoned upload artifacts require future cleanup/retention policy;
- only sequential chunks are supported in this version;
- cross-node upload continuation is not provided until cluster storage semantics exist.

## Deferred

- automatic expiry/cleanup of abandoned sessions;
- parallel/multipart chunk upload;
- upload bandwidth policy;
- deduplication/content-addressed storage;
- cross-node transfer continuation.
