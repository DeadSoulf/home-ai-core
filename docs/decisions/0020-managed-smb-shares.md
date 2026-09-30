# ADR-0020: Managed SMB shares for Home-AI NAS

## Status

Accepted.

## Context

Home-AI NAS already provides logical private/shared folders, scoped Web/API access and a controlled physical storage boundary. Windows clients need a standard LAN file protocol before the dedicated Home-AI Windows copy/sync client exists.

SMB must not bypass the NAS namespace, expose arbitrary host paths, enable guest access or require Core to store plaintext/reversible user passwords.

## Decision

Home-AI manages Samba as an optional NAS transport.

### Installation

Samba is installed only after an explicit owner action.

The privileged updater helper performs package installation through the existing transient systemd package-install path and enables `smbd.service`.

Samba is not required for the Core to start and is not a mandatory dependency of normal update bundles.

### Credentials

SMB credentials are separate from Home-AI login credentials.

For each active Home-AI user, Core derives a stable internal Unix/Samba username from the stable Home-AI user ID. The mapping is deterministic but does not expose or depend on the Home-AI password.

The owner explicitly sets or replaces the SMB password through the Web UI. The password is sent to the privileged helper only for the requested operation and is not persisted in SQLite, logs, Audit metadata or Core configuration.

Disabled Home-AI users are excluded from generated SMB shares.

### Shares

Only existing Home-AI logical NAS folders are exported.

Physical share paths are resolved through the existing `filedata.FolderRoot` boundary and the privileged helper validates that the path:

- is under `/mnt/home-ai-core`;
- is inside a Home-AI `.home-ai` namespace;
- contains no symlink path component;
- is a real directory;
- is owned by the Home-AI Core service identity.

The initial implementation mapped private/shared semantics directly. ADR-0031 supersedes that ACL mapping now that the unified access editor exists.

The current access mapping is derived from the same effective Core authorization used by Web/API:

- `files.manage`: read/write access to every generated share;
- global `files.read` / `files.write`: the corresponding access to every folder;
- scoped `files.read` / `files.write` on a `file_folder`: access only to that exact folder;
- disabled users: excluded.

Private-folder owners keep their direct scoped grants created with the folder. Shared folders receive no automatic all-household grant; the administrator selects users explicitly in the unified Users access editor.

This keeps SMB and HTTP authorization aligned instead of maintaining a second ACL policy.

### Samba process identity

Samba authentication uses the generated per-user SMB accounts, but filesystem operations are forced to:

```text
home-ai-core:home-ai-core
```

This preserves the current NAS physical ownership model and keeps authorization at the Samba share layer.

### Security defaults

Generated Home-AI configuration enforces:

- no guest access;
- `map to guest = Never`;
- SMB2.10 minimum protocol;
- explicit `valid users`;
- explicit write lists;
- `follow symlinks = no`;
- `wide links = no`;
- controlled file/directory masks;
- hidden Home-AI recycle/upload artifacts.

### Configuration ownership

Home-AI owns:

```text
/etc/samba/home-ai.conf
```

The existing distribution/operator `/etc/samba/smb.conf` is not replaced. Home-AI adds one marked include block if it is absent, after creating a timestamped backup.

The generated config is validated with `testparm` before activation. If complete-config validation fails after replacement, the previous Home-AI managed config is restored.

### API

Administrative SMB status and operations live under:

```text
GET  /api/v1/files/smb
POST /api/v1/files/smb/operation
```

They require `files.manage`; cookie writes require CSRF.

Mutations are audited without password material.

## Consequences

Benefits:

- Windows can access the same logical NAS folders as the Web/API;
- no guest shares;
- no arbitrary host path exports;
- Home-AI passwords are not reused or exposed to Samba;
- Core does not persist SMB passwords;
- existing Samba configuration is retained through a managed include;
- the physical NAS ownership model remains unchanged.

Trade-offs:

- users have a separate SMB credential;
- generated SMB usernames are internal identifiers rather than the Home-AI login name;
- SMB ACL generation now depends on the unified Core permission/resource model defined by ADR-0031;
- live Samba installation/connectivity still requires acceptance testing on the installed server.

## Deferred

- disable/remove SMB credential when a Home-AI user is disabled/deleted;
- SMB quotas;
- SMB snapshots/Previous Versions integration;
- remote SMB exposure (direct public SMB remains prohibited);
- Windows Home-AI copy/sync client.
