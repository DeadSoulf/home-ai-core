# ADR-0033: NAS pool free-space reserve and capacity warnings

## Status

Accepted for implementation, 2026-10-01.

## Context

Home-AI now binds each NAS pool to a persistent backing filesystem identity, but file writes could still consume all available filesystem space. Exhausting the backing filesystem can destabilize file services and other data-plane operations that share the same filesystem.

Capacity protection must apply to Web uploads and the Windows client without relying on UI-only checks. The current SMB data path is different: Samba writes directly to the filesystem and does not pass through the Core upload API.

## Decision

Each NAS pool stores two capacity-policy percentages:

- `reserve_percent`: hard free-space reserve. Default: 5%.
- `warning_percent`: informational low-space threshold. Default: 10%.

Migration 016 applies those defaults to existing pools. Administrators can edit both values in the Files Web UI. The reserve may be set from 0 to 50%. The warning may be set from 0 to 95%; zero disables the warning, otherwise it must not be below the reserve.

Core reads current filesystem capacity with Linux `statfs` at the pool root. Capacity shown in the Files UI is therefore derived from the real mounted filesystem rather than the slower storage inventory cache.

For writes that materially consume capacity:

- direct file uploads are limited before their temporary file can cross the reserve;
- resumable upload creation is rejected if the declared total cannot fit above the reserve;
- every resumable chunk re-checks current free space, preventing concurrent sessions from relying only on an earlier estimate.

The API returns HTTP 507 with code `file_pool_reserve_reached` when a write would violate the reserve. If current capacity cannot be read, capacity-consuming API writes fail closed with `file_pool_capacity_unavailable`.

Moves, recycle-bin moves, restores and renames stay allowed because they are same-filesystem renames and do not materially increase occupied bytes.

## SMB boundary

Direct SMB writes currently bypass Core's upload API. The Web UI explicitly states that the pool reserve is not yet a hard filesystem quota for those writes.

ADR-0034 implements the follow-up boundary with kernel user-quota enforcement for quota-ready ext4/XFS backing filesystems. Legacy filesystems remain explicitly not ready until a controlled migration/remount/reformat enables quota accounting.

## Consequences

Web and Windows-client writes share one server-side capacity policy and cannot silently fill the filesystem below the configured reserve. Administrators get a visible early-warning threshold before writes are blocked.

The policy is intentionally pool-wide. Per-folder and per-user quotas remain a separate layer and should be built on top of this pool safety boundary.
