# Managed storage binding for Docker modules (NVR first)

Status: implementation design; **not yet enabled**. This document intentionally does not change the installed Docker runtime.

## Observed fault (2026-10-18)

On node `homeaicore`, `storage_purposes` records:
- `/dev/sdc1`, filesystem UUID `6740f386-7f43-4f9a-be9f-617cf7899d67`, purpose `nvr`;
- filesystem mounted at `/mnt/home-ai-core/sdc1`;
- `home-ai-module-nvr.module` binds `/var/lib/home-ai-modules/nvr.module` to `/data` instead.
Thus `df -h /data` reports system `/dev/sda1`, not the assigned 3.1 TB disk.

The existing SDK v2 runtime deliberately grants only the persistent `/data` bind. Do not allow arbitrary mounts via manifest or user-supplied paths.

## Proposed safe contract

1. Add an explicit Core-owned mapping of module ID to storage purpose (initially `nvr.module -> nvr`), with a typed, validated helper request. Never infer a host bind from untrusted module manifest fields.
2. Resolve purpose to the registered filesystem UUID via `storage_purposes`; treat `device_path` as informational (device names may change).
3. In the privileged helper, revalidate the source *immediately before container creation/start*: mounted filesystem, exact filesystem UUID, allowed location under `/mnt/home-ai-core`, directory permissions and container UID/GID write access. Do not follow symlinks outside the approved root or let Docker auto-create the source path.
4. Preserve separate module configuration/state on its existing Core-managed bind. Put recordings on a purpose-specific directory **on the selected filesystem** mounted to a separate declared container path. Before choosing that path, audit the NVR image's existing use of `/data` so that databases/config files and archive paths are not accidentally split or hidden.
5. On a missing, unmounted or mismatching disk, **fail closed**: return a visible storage-unavailable module status, prevent recordings, and never write to an identically named directory on the root filesystem. Validate again on restart and after upgrade.
6. For existing installations, stage a one-time migration of recordings if necessary, with a stop/backup/copy/verify/rollback process; never remove existing /data contents automatically.
7. Report capacity via filesystem statfs for the **actual recordings mount** (total/free/available/used bytes). Expose mount identity and storage health to NVR UI through a protected Core/SDK interface; treat quota and unprivileged available blocks consistently.
8. Integrate with installer, reinstallation, reboot and helper-driven module lifecycle; do not grant Docker socket or root privileges to network-facing Core.

## Tests required before enabling

- NVR's assigned disk mounted with correct UUID -> recordings mount resolves to /dev/sdc1, status correct.
- Disk missing, plain directory left behind, wrong UUID, symlink escape, unmounted filesystem -> installation/restart is blocked.
- Other modules unaffected, including `reference.module`; existing `/data` contract remains intact.
- Container recreate and Core reboot retain assignment; device renumbering uses UUID rather than `/dev/sdc1`.
- Failure during migration leaves prior data intact; repeated request idempotent.
- Reported NVR disk capacity equals host `df` for mounted filesystem and not `/dev/sda1`.
- Permission failures, exhausted disk and disappearing mount are visible in UI and do not cause fallback writes.

## Implementation checkpoints

- [ ] Locate Docker run/create implementation in privileged helper and document current NVR data layout.
- [ ] Introduce restricted module-to-purpose lookup and validated helper protocol.
- [ ] Add mountpoint/UUID enforcement plus fail-closed runtime checks.
- [ ] Add NVR recording-path integration and capacity endpoint/UI.
- [ ] Add lifecycle/migration tests and CI.
- [ ] Publish signed release, then deploy in a controlled maintenance window.

No production changes are implied by this document.
