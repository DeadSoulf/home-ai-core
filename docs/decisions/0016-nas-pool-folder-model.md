# ADR-0016: NAS Pool and Folder Resource Model

- Status: Accepted
- Date: 2026-09-29

## Context

Home-AI needs a native file-storage domain with:

- storage spaces backed by local storage;
- private folders for individual users;
- shared household folders;
- scoped permissions;
- a future Web file manager;
- SMB/NFS exposure;
- a future Windows file-copy client.

Low-level disk/partition/filesystem operations already exist, and resource-scoped permissions already support grants to concrete resource IDs.

## Decision

Introduce two logical NAS resources.

### File pool

A file pool is a logical storage root:

- stable ID;
- display name;
- absolute root path;
- creator metadata.

The first version stores metadata only. It does not yet create directories or validate that the root path belongs to a mounted data filesystem.

That validation/provisioning is the next NAS slice.

### File folder

A file folder belongs to one pool and has:

- stable ID;
- display name;
- kind: `private` or `shared`;
- optional owner user;
- stable relative path;
- creator metadata.

Physical paths are derived from stable IDs rather than display names so renaming a folder later does not change its storage identity.

Private folders use:

```text
users/<user-id>/<folder-id>
```

Shared folders use:

```text
shared/<folder-id>
```

### Permissions

Introduce:

- `files.read`;
- `files.write`;
- `files.manage`.

The owner role receives all three globally.

A private folder automatically grants the selected user scoped `files.read` and `files.write` on:

```text
resource_type = file_folder
resource_id   = <folder-id>
```

A shared folder automatically grants the `member` role scoped read/write access to that folder.

Members do not receive global file permissions.

### API exposure

Any authenticated household account can list folders, but the response is filtered by effective resource permissions.

Only accounts with `files.manage` can create/list pools or create folders.

## Consequences

The file domain can grow without coupling identity to filesystem paths.

Future features can build on the same stable folder ID:

- real directory provisioning;
- file operations;
- quotas;
- SMB shares;
- NFS exports;
- snapshots;
- AI file tools;
- per-folder policy changes.

## Deferred

This ADR deliberately does not define yet:

- filesystem provisioning;
- mounting policy;
- storage-pool capacity allocation;
- file upload/download API;
- folder deletion;
- quota enforcement;
- SMB/NFS implementation;
- per-user custom shared-folder grants.
