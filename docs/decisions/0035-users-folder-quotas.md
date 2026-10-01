# ADR-0035: Users, folder quotas and access reconciliation

Status: accepted for 0.1.91-dev

## Context

The shared Core identity and physical NAS foundation are already present. User edits must preserve stable ownership, while folder, user and capacity policies must govern both API transfers and separately authenticated Samba access. A static Samba snapshot or long-lived realtime socket must not preserve revoked rights.

## Decision

- Keep the Core user ID, private owner ID, folder ID and physical relative path stable when changing visible names or login. Replace a user's grants without removing their personal-folder ownership. Disabling retains files and ownership but revokes sessions. Protect the last enabled administrator in the state transaction.
- Changing a password requires the current password for self-service; administrative reset targets other accounts. Password changes atomically revoke all sessions. Login changes also revoke sessions; display-name-only changes do not. SMB credentials remain separate credentials tied to the stable identity, and their passwords are not stored by Core.
- Migration 017 adds per-folder logical/hard caps, private-user aggregate caps, persistent workgroup settings and monotonically allocated project IDs. A deleted/provisioning-failed folder's project ID is never reused. An administrator edits folder names, direct grants and quotas in Files, and personal caps in Users.
- A Core folder quota counts apparent file sizes including trash, plus declared resumable-upload reservations; parts are not counted twice. The private-user cap sums only their private folders across all pools. Shared folders have separate caps and are not charged to an arbitrary uploader. Lowering a cap below committed/reserved usage is rejected. Global mutation serialization protects cross-pool reservations and refreshes authorization after queued requests resume.
- Filesystem project IDs use the reserved offset 1,000,000 plus the persisted ID. Finite SMB folder caps require enforced ext4/XFS project quotas, inherited by new directories/files. The helper refuses unsafe paths, symlinks, special files, multiply linked files or foreign projects, adopts existing files, verifies allocated usage, applies the hard cap and reads it back. The cap rounds down to KiB and must be at least 1 MiB. Kernel quotas count allocated blocks; sparse file apparent sizes may differ from Core accounting.
- The existing Core-UID kernel quota from ADR-0034 retains the pool reserve for independent Samba writes. Unlimited folders keep that behavior. Finite folder/personal caps require verified project caps; otherwise those folders are read-only over SMB. A finite personal cap is allocated across every owned private folder, so the sum of hard caps cannot exceed it. Core keeps space promised to pending uploads and other filesystem project allocations, with filesystem metadata headroom.
- Before changing account grants, folder policy or pool capacity, stop Samba and its existing handles, replace managed configuration with an empty safe snapshot, persist authoritative policy, then apply a fresh derived snapshot. Failed suspension prevents mutation. Failed refresh leaves access suspended and returns a visible warning. Reconcile on helper/Core startup. This intentionally interrupts managed SMB connections during permission edits.
- Uploaded bytes are attachments with octet-stream, nosniff and a restrictive sandbox. Realtime reauthenticates before commands/delivery and once a second while idle; folder events and history require current folder read access. Topic limits apply to each connection's union, with atomic rejection.
- Files has three task-oriented tabs: folders, storage and Windows access. Account has self-service password change. Capabilities and administrative reset are expandable in Users. The disk-first System UI from 0.1.90-dev remains intact.

## Filesystem preparation and boundaries

New explicitly formatted ext4 data filesystems receive both user and project quota features; mounting uses user quotas and project quotas when supported. XFS mounting enables both. Existing filesystems are not reformatted or silently converted. Older filesystems need an explicit operational preparation before enabling finite hard caps. No target server disks were changed during development.

Core's service namespace permits NAS writes under `/mnt/home-ai-core`; the updated helper reconciles that systemd drop-in for existing bundle installations. Other host writers and filesystem metadata are outside per-folder product accounting. NFS and snapshot/backup policies remain optional future transports/features.

## Validation

Regression tests cover concurrent reservations, private ownership, last-admin races, credential/session lifecycle, hostile download headers, live socket revocation and private events, Samba suspension/failure ordering and capacity allocations. An opt-in test on a disposable ext4 loop image verifies a non-root writer is denied beyond the project cap and can write again after the cap is cleared. The browser fixture uses disposable users/files and never invokes real Samba or the privileged broker. Installed-server SMB, reboot and Windows/NAS acceptance remain separate.

## API additions

- `PUT /api/v1/auth/password`: current/new password; returns 204 and clears the session cookie.
- `PUT /api/v1/security/users/{id}/identity`: login/display name.
- `PUT /api/v1/security/users/{id}/password`: administrative reset of another account.
- `GET /api/v1/files/owners`: minimal enabled-owner picker.
- `GET|PUT /api/v1/files/folders/{id}/settings`: name, quota bytes, filesystem enforcement and direct grants.
- `GET /api/v1/files/quotas`; `PUT /api/v1/files/users/{id}/quota`: private-user aggregate limits.

All writes use the existing authorization and cookie CSRF contract. Quota/reserve rejection is HTTP 507 (`file_quota_or_reserve_reached`, or the existing pool-reserve code when no folder/personal cap applies). Settings changes return a warning when managed SMB remains suspended.
