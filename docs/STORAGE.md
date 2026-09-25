# Home AI Core — Storage Monitoring and Disk Operations

## Purpose

The Storage subsystem provides a live view of filesystems and block devices that Home AI Core can use for:

- video surveillance archives
- personal files
- system storage
- future backups and AI data

It also supports hot-plug discovery and a guarded disk-management workflow.

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

## Hot-plug / new disk discovery

The monitor scans Linux sysfs under:

```text
/sys/class/block
```

The Web interface checks the list automatically every few seconds and also exposes:

```text
Проверить новые диски
```

For a new or currently unused block device the UI shows:

- device path
- disk/partition type
- model/vendor where available
- serial number where available
- size
- removable/hot-plug indication
- a button: `Управление диском`

## Disk management menu

The disk-management dialog contains standard storage actions.

### Non-destructive role actions

For mounted filesystems:

- assign as camera-video storage
- assign as personal-file storage
- remove Home AI storage assignment
- refresh information

### Privileged storage actions

When the Storage Helper is installed:

- mount for camera video
- mount for personal files
- unmount a Home AI managed filesystem
- format the selected unused device as EXT4
- remove filesystem/partition signatures with `wipefs`

Home AI mounts managed disks below:

```text
/mnt/home-ai/video/
/mnt/home-ai/files/
```

Mounted filesystems use conservative options:

```text
nodev,nosuid,noexec
```

## Destructive-operation protection

Formatting and signature removal require all of the following:

1. authenticated administrator session
2. device must exist in the current Linux block inventory
3. device must not be mounted
4. device must not be active swap
5. device must not be held by LVM/device-mapper or another Linux block layer
6. device must be an eligible unused disk/partition
7. the user must type the exact device path, for example `/dev/sdb`, into a confirmation prompt

The Web Core never passes a shell command assembled from user text. The privileged helper receives validated arguments and executes a small fixed allow-list of system utilities.

## Privileged Storage Helper

The main Home AI Core process should remain unprivileged.

Disk mount/format/unmount operations are delegated to:

```text
/usr/local/libexec/home-ai-storage-helper
```

The helper must be installed once after building:

```bash
sudo sh scripts/install-storage-helper.sh
```

When run through `sudo`, the installer detects the invoking user through `SUDO_USER`. A user can also be supplied explicitly:

```bash
sudo sh scripts/install-storage-helper.sh texnik
```

The installer:

- installs the helper as root-owned executable
- creates `/mnt/home-ai/video`
- creates `/mnt/home-ai/files`
- installs a narrowly scoped sudo rule allowing only the Home AI storage helper

The helper itself performs the block-device safety checks again before every privileged action.

## Storage roles

Two configuration keys track active mount points:

```text
storage.video_mounts=
storage.personal_mounts=
```

Example:

```text
storage.video_mounts=/mnt/home-ai/video/sdb1
storage.personal_mounts=/mnt/home-ai/files/sdc1
```

## Offline detection

Configured video/personal mount points remain visible even when not mounted and are shown as:

```text
OFFLINE
```

This lets the future NVR and personal-data services detect missing storage before writing data.

## Web API

Authenticated users can read mounted storage:

```text
GET /api/storage
```

and block-device inventory:

```text
GET /api/storage/devices
```

Administrator disk actions use:

```text
POST /api/storage/action
```

Supported action names currently include:

```text
mount-video
mount-personal
assign-video
assign-personal
unassign
unmount
format-ext4
wipefs
```

## Safety filtering

A device is not offered as a new storage candidate when Home AI Core can see that it is already:

- mounted
- used as swap
- held by another Linux block layer such as LVM/device-mapper

Whole disks that already contain partitions are not offered directly; eligible unused partitions are considered instead.

## Planned extensions

The full Storage Core will later add:

- persistent disk identity by UUID
- filesystem type detection before mounting
- GPT/partition creation wizard
- disk labels and rename operations
- safe eject / power-off where supported
- HDD / SSD / NVMe classification
- SMART health
- disk temperature
- wear / lifetime indicators
- recording retention limits
- minimum free-space thresholds
- NVR disk rotation
- personal-data quotas
- storage pools
- RAID / mirror awareness
- disk failure events through Event Bus


## Web API output safety

Storage helper output can contain terminal control bytes generated by utilities such as
`mkfs.ext4`. All helper messages returned through JSON are escaped according to JSON string
rules, including backspace, form-feed, ESC and every other byte below `0x20`.

This prevents disk-format results from breaking `response.json()` in the Web UI.
