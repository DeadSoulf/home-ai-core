# Hardware and Storage Discovery

Home-AI-Core keeps descriptive hardware discovery in the unprivileged Core and routes privileged storage inspection/mutation through the root helper.

## Unprivileged Core sources

Core reads safe host information from Linux and standard APIs, including:

- `/proc/cpuinfo`
- `/proc/meminfo`
- `/proc/uptime`
- `/sys/block`
- `/sys/class/net`
- `/sys/class/drm`
- `lsblk` for the recursive block-device tree
- the PCI ID database when available

The resulting inventory includes CPU, memory, network, GPU/PCI and block-device metadata.

## Privileged storage inspection

The unprivileged Core does not open arbitrary block devices itself. It asks `home-ai-core-updater` over the authenticated Unix socket for bounded storage inspection.

The helper currently provides:

- filesystem free-space information, including supported unmounted filesystems
- SMART health and temperature
- power-on hours where SMART exposes them
- SSD/NVMe lifetime where available
- LVM volume-group/logical-volume metadata

SMART checks use standby-safe probing so routine UI refreshes do not intentionally wake sleeping drives.

## Storage mutation

Storage v1 supports guarded operations on non-system disks:

- partition create/delete/delete-all
- filesystem creation: ext4, XFS and FAT
- filesystem-label changes
- mount/unmount under the Home-AI-Core mount root
- active swap shutdown before destructive operations
- LVM volume-group deactivation when the group is fully contained on the target physical disk

The helper rejects destructive operations on the physical disk that contains the running root filesystem.

## Identity

Kernel names such as `sda` and `nvme0n1` remain technical paths, not durable user identity.

Home-AI-Core can persist a user-facing physical-disk name using the disk serial when available, with the device path only as a fallback stable key.
