# Home AI Core — Storage Monitoring

## Purpose

The Storage Monitor provides a live view of filesystems and block devices that Home AI Core can use for:

- video surveillance archives
- personal files
- system storage
- future backups and AI data

It also provides the first hot-plug discovery flow for newly attached disks.

## Mounted storage discovery

The monitor reads mounted block-device filesystems from Linux and reports:

- device source
- mount point
- filesystem type
- ONLINE / OFFLINE state
- read-only state
- total capacity
- used capacity
- free capacity
- used percentage
- assigned Home AI Core role

Only block-device backed mounts are treated as local disk storage.

## Hot-plug / new disk discovery

The monitor also scans Linux sysfs under:

```text
/sys/class/block
```

The Web interface checks this list automatically every few seconds and also exposes a manual button:

```text
Проверить новые диски
```

For a new or currently unused block device the UI can show:

- device path
- disk/partition type
- model/vendor where available
- serial number where available
- size
- removable/hot-plug indication
- suggested actions

Current suggested actions:

- use for camera video
- use for personal files
- do not use for now

The current implementation intentionally does not format or mount a disk automatically. Selecting a role only records the intended action in the UI. A later Storage Core step will add a privileged preparation workflow with explicit destructive-operation confirmation.

## Safety filtering

A device is not offered as a new storage candidate when Home AI Core can see that it is already:

- mounted
- used as swap
- held by another Linux block layer such as LVM/device-mapper

Whole disks that already contain partitions are not offered directly; eligible unmounted partitions are considered instead.

This reduces the risk of presenting an in-use system disk as available storage.

## Storage roles

Two configuration keys assign storage roles:

```text
storage.video_mounts=
storage.personal_mounts=
```

Values are comma-separated mount points.

Example:

```text
storage.video_mounts=/mnt/video1,/mnt/video2
storage.personal_mounts=/mnt/files1
```

A mount point may be assigned to both roles.

The system root filesystem is identified as `system`. Mounted disks without an assigned role are shown as `unassigned`.

## Offline detection

Configured video/personal mount points remain visible even when they are not mounted.

In that case the dashboard reports:

```text
OFFLINE
```

This lets the future NVR and personal-data services detect missing storage before writing data.

## Web API

Authenticated users can read mounted storage:

```text
GET /api/storage
```

and the block-device inventory / hot-plug candidates:

```text
GET /api/storage/devices
```

The block-device response includes fields such as:

```text
device
type
model
vendor
serial
size_bytes
removable
mounted
in_use
has_partitions
candidate
```

## Dashboard

The Web Core refreshes mounted storage every five seconds and new-device discovery every three seconds.

When a newly appearing candidate is detected while the page is open, the dashboard displays a notification and offers actions.

## Planned extensions

The full Storage Core will later add:

- privileged disk preparation with explicit confirmation
- filesystem detection and creation
- safe mount/unmount workflow
- persistent storage-role assignment by UUID
- HDD / SSD / NVMe classification
- SMART health
- disk temperature
- wear / lifetime indicators where supported
- recording retention limits
- minimum free-space thresholds
- NVR disk rotation
- personal-data quotas
- storage pools
- RAID / mirror awareness
- disk failure events through Event Bus
