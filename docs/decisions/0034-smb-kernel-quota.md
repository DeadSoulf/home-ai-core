# ADR-0034: Kernel-enforced SMB pool reserve

## Status

Accepted for implementation, 2026-10-01.

## Context

ADR-0033 protects Core API and Windows-client uploads from consuming the configured NAS pool free-space reserve. Samba is a separate data path and writes directly to the filesystem.

Home-AI already renders every managed Samba share with:

- `force user = home-ai-core`
- `force group = home-ai-core`

Therefore every direct SMB write is attributed by the kernel to the Home-AI service UID, even though SMB authentication and folder permissions remain per Home-AI user.

A Samba-only free-space display is not sufficient. The server must reject writes at the filesystem layer when they would consume capacity reserved by the pool policy.

## Decision

Home-AI uses Linux user quota accounting for the `home-ai-core` service UID on quota-ready ext4 and XFS backing filesystems.

The quota hard limit is not a fixed percentage of total filesystem size. Existing data outside `.home-ai` may already consume capacity. For a pool with a non-zero reserve, Core calculates the safe hard limit as:

`current Home-AI UID usage + max(0, current filesystem free space - configured reserve bytes)`.

This means existing non-Home-AI data is included in the free-space calculation. Normal Home-AI writes increase quota usage and decrease free space by approximately the same amount, so the safe maximum remains stable while other filesystem usage is unchanged.

If external filesystem usage changes, SMB inspection recalculates the safe maximum. A configured hard limit greater than that safe maximum is reported as out of sync instead of being presented as protected.

### Filesystem readiness

New Home-AI ext4 filesystems are formatted with embedded user quota support and no separate ext4 root-reserved percentage:

`mkfs.ext4 -F -m 0 -O quota -E quotatype=usrquota`.

Home-AI mounts:

- ext4 with `usrquota`;
- XFS with `uquota`.

This avoids silently stacking the explicit Home-AI reserve on top of a second ext4 root reserve for newly created storage.

Existing filesystems are not live-remounted or rewritten automatically. Legacy ext4 must have the embedded quota feature and active user-quota accounting. Legacy XFS must have user quota accounting enabled at mount time. Otherwise SMB status reports the storage as not quota-ready and explains that controlled migration/remount/reformat is required.

### Enforcement

The privileged helper owns quota changes.

Before `smb.apply` writes or reloads Samba configuration, it:

1. validates each share's pool root and reserve policy;
2. verifies the backing filesystem is quota-ready;
3. reads current service-UID usage with `repquota`;
4. computes the safe hard limit from live free space;
5. applies the block hard limit with `setquota`;
6. reads the quota back and verifies the applied value.

If any pool with a non-zero reserve cannot be protected, `smb.apply` fails closed before the Samba configuration is activated.

Changing a NAS pool reserve also asks the privileged helper to synchronize the kernel quota immediately. Database/API policy remains valid even if a legacy filesystem is not ready; the SMB status then remains explicitly not ready until the storage is migrated or repaired.

A reserve of zero disables the managed block hard limit where quota accounting is available.

### Status

`smb.inspect` reports whether all exported non-zero-reserve pools have a kernel limit that is no greater than the currently safe maximum. The Web Files page surfaces this separately from Samba service availability.

## Scope boundary

This is a pool safety limit, not a per-user storage allowance. Samba authentication users are still forced to the common service UID for filesystem access, so this quota cannot distinguish household users.

Per-folder and per-user quotas remain a later layer. A future design may use project quotas, dedicated filesystem ownership identities, or another mechanism depending on the supported filesystem matrix.

Files on the same backing filesystem that are owned by `home-ai-core` but live outside `.home-ai` also count toward this service-UID quota. Home-AI therefore keeps service-owned pool data inside its managed `.home-ai` area.

## Consequences

Direct SMB writes now have a kernel-enforced stop on supported quota-ready filesystems instead of relying on client-side free-space reporting.

New Home-AI ext4/XFS mounts are prepared for this enforcement. Existing filesystems remain safe from automatic destructive conversion, but may require an explicit maintenance migration before hard SMB reserve protection becomes active.
