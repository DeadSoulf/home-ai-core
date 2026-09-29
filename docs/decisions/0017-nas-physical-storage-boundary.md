# ADR-0017: NAS Physical Storage Boundary

- Status: Accepted
- Date: 2026-09-29

## Context

The first NAS slice introduced logical pools and folder resources, but did not create anything on disk.

Home-AI-Core itself runs unprivileged while storage provisioning requires controlled root access. The existing updater/storage helper already provides an authenticated root boundary over a Unix socket.

The NAS layer must not turn an arbitrary host directory or the system filesystem into managed storage by accident.

## Decision

### Pool root

A NAS pool root must be:

- an absolute path;
- a real directory, not a symlink;
- below `/mnt/home-ai-core/`;
- the filesystem mount point itself;
- writable.

The privileged helper verifies the mount point with `findmnt` and filesystem flags before accepting the pool.

This deliberately excludes:

- `/`;
- `/etc`, `/var`, `/home` and other host directories;
- arbitrary subdirectories of a mounted filesystem;
- symlink paths;
- read-only filesystems.

### Home-AI data namespace

Home-AI owns only this directory inside an accepted pool root:

```text
<pool-root>/.home-ai/
```

The initial layout is:

```text
.home-ai/
  users/
    <user-id>/
      <folder-id>/
  shared/
    <folder-id>/
```

Existing content outside `.home-ai` is not modified.

### Stable paths

Physical directory names use stable internal resource IDs rather than user-visible names.

Allowed folder-relative paths are restricted to exactly:

```text
shared/<folder-id>
users/<user-id>/<folder-id>
```

Path traversal and arbitrary relative paths are rejected.

### Ownership and permissions

Directories created by the privileged helper are owned by the UID/GID of the authenticated Home-AI-Core Unix-socket peer.

The initial directory mode is `0750`.

This allows the unprivileged Core process to perform future file operations inside its own NAS namespace without running the network-facing service as root.

### Privileged boundary

The helper protocol remains version 2.

New optional fields carry:

- pool root path;
- stable relative folder path.

The helper owns validation and directory creation. The Core never receives broad root filesystem access.

### Metadata consistency

Pool creation validates/provisions the physical pool before recording pool metadata.

Folder creation first creates metadata and scoped permission grants, then provisions the physical folder. If folder provisioning fails, Home-AI removes the newly-created folder metadata and its scoped grants.

## Consequences

The next file-management layer can safely resolve every logical folder to:

```text
<pool-root>/.home-ai/<relative-path>
```

without trusting browser-supplied host paths.

The same stable folder identity can later back Web file operations, SMB shares, quotas, snapshots and AI file tools.

## Deferred

This ADR does not yet define:

- file CRUD/upload/download operations;
- quotas;
- filesystem snapshots;
- SMB/NFS exports;
- ACL mapping to OS users;
- pool deletion;
- moving a logical folder between pools.
