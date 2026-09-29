# ADR-0018: NAS File Data Boundary

- Status: Accepted
- Date: 2026-09-30

## Context

Home-AI now has logical storage pools and folder resources with scoped permissions, plus physical directories provisioned under a mounted data filesystem.

The next step is file content access. File data must not be stored in SQLite and must never escape a user's authorized logical folder through path traversal or symbolic links.

## Decision

File metadata that belongs to the control plane remains in SQLite.

User file contents live only on the selected NAS filesystem under:

```text
<pool-root>/.home-ai/<logical-folder-relative-path>
```

All file operations first resolve the logical folder resource and effective user permissions.

### Authorization

Read operations require either:

- global `files.manage`; or
- scoped `files.read` for the target `file_folder`.

Write operations require either:

- global `files.manage`; or
- scoped `files.write` for the target `file_folder`.

### Path safety

All user-supplied paths are relative to the logical folder root.

The file service rejects:

- absolute paths;
- `..` traversal;
- symlink traversal in any existing path segment;
- opening a symlink as a regular file;
- creating children through a symlink.

The logical folder root itself must be a real directory.

### Uploads

Uploads are streamed to a temporary file inside the target directory, fsynced, closed and then atomically renamed to the requested file name.

The initial API limits a single upload to 512 MiB.

Internal temporary upload files are hidden from directory listings.

### Downloads

Downloads use the standard Go `http.ServeContent` path after authorization and safe resolution. This preserves range requests and normal HTTP caching semantics without exposing physical paths.

### Initial operations

The first file data slice supports:

- list directory entries;
- create directory;
- upload/replace a regular file;
- download a regular file.

Delete/move/rename are intentionally deferred to the next slice because they need additional destructive-action audit and overwrite semantics.

## Consequences

- file bytes stay out of the control-plane database;
- scoped folder permissions apply consistently to file data;
- directory traversal and symlink escape are blocked;
- uploads do not expose partial target files;
- later SMB/NFS can reuse the same physical folder layout.

## Deferred

- delete;
- move;
- rename;
- recursive operations;
- trash/recycle bin;
- resumable/chunked uploads;
- quotas;
- checksums;
- SMB/NFS export.
