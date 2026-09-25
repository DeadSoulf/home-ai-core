# Home AI Core — Storage Monitoring

## Purpose

The Storage Monitor provides a live view of filesystems that Home AI Core can use for:

- video surveillance archives
- personal files
- system storage
- future backups and AI data

This is the monitoring foundation for the later full Storage Core.

## Discovery

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

Only block-device backed mounts (sources under `/dev/`) are treated as local disk storage.

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

Authenticated users can read:

```text
GET /api/storage
```

Example response shape:

```json
{
  "volumes": [
    {
      "source": "/dev/sdb1",
      "mount_point": "/mnt/video1",
      "filesystem": "ext4",
      "role": "video",
      "status": "online",
      "total_bytes": 1000000000000,
      "used_bytes": 250000000000,
      "free_bytes": 750000000000,
      "used_percent": 25.0,
      "read_only": false
    }
  ]
}
```

## Dashboard

The Web Core refreshes disk information every five seconds.

Each storage card shows:

- assigned role
- status
- device
- mount point
- filesystem
- used / total capacity
- free capacity
- read-only warning

## Planned extensions

The full Storage Core will later add:

- disk model and serial inventory
- HDD / SSD / NVMe classification
- SMART health
- disk temperature
- wear / lifetime indicators where supported
- automatic storage-role assignment
- recording retention limits
- minimum free-space thresholds
- NVR disk rotation
- personal-data quotas
- storage pools
- RAID / mirror awareness
- disk failure events through Event Bus
