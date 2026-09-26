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

The disk-management dialog uses role checkboxes rather than separate mount/assign actions.
A disk can be assigned to:

- video
- home files
- virtual machines
- any combination of these roles

For an unused unmounted disk, Home AI Core mounts it once and adds the resulting mount point
to every selected pool. For an already mounted disk, only the Home AI role assignment changes.

### Privileged storage actions

When the Storage Helper is installed:

- mount a new managed disk
- unmount a Home AI managed filesystem
- format the selected unused device as EXT4
- remove filesystem/partition signatures with `wipefs`

New managed mount points use the filesystem UUID when available:

```text
/mnt/home-ai/video/<filesystem-uuid>
/mnt/home-ai/files/<filesystem-uuid>
```

Legacy device-name mount points remain readable for compatibility.

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
- creates `/mnt/home-ai/vm`
- installs a narrowly scoped sudo rule allowing only the Home AI storage helper

The helper itself performs the block-device safety checks again before every privileged action.

## Storage roles and pools

Three comma-separated configuration keys track all mount points in each pool:

```text
storage.video_mounts=
storage.personal_mounts=
storage.vm_mounts=
```

The same mount point may exist in both keys, so one physical disk can serve both roles.

Placement policies are configured independently for video and files:

```text
storage.video_policy=most_free
storage.files_policy=most_free
storage.video_reserve_percent=10
storage.files_reserve_percent=10
storage.video_reserve_gb=0
storage.files_reserve_gb=0
storage.video_pinned_mount=
storage.files_pinned_mount=
```

Supported policies:

- `most_free` — choose the eligible disk with the most usable free space
- `sequential` — fill eligible disks in deterministic order
- `balanced` — choose the disk with the lowest used percentage
- `pinned` — prefer the configured mount point, then fall back safely if it is unavailable

The effective reserve on each volume is the larger of the configured percentage and GB reserve.
Volumes that have reached that reserve are excluded from new-write selection.

## Offline detection

Configured video/personal/VM mount points remain visible even when not mounted and are shown as:

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

Supported action names include the current role-based action:

```text
apply-roles
unmount
format-ext4
wipefs
```

Legacy mount/assign action names remain accepted for compatibility with older clients.

## Safety filtering

A device is not offered as a new storage candidate when Home AI Core can see that it is already:

- mounted
- used as swap
- held by another Linux block layer such as LVM/device-mapper

Whole disks that already contain partitions are not offered directly; eligible unused partitions are considered instead.

## Implemented storage-pool foundation

The current development branch includes:

- persistent filesystem UUID discovery
- UUID-based mount paths for newly managed disks
- automatic best-effort remount of UUID-managed volumes after service startup
- backward-compatible startup remount for existing legacy device-name mount paths
- multi-disk video and files pools
- dual-role disks
- pool write-target selection
- percent/GB reserve thresholds
- most-free, sequential, balanced and pinned placement policies

## Planned extensions

The full Storage Core will later add:

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


## Disk role workflow

The Web UI now exposes three role checkboxes and one `Применить назначение` action.
This removes the ambiguous distinction between "mount" and "assign" buttons while still
preserving the same safe backend behavior.


## VM Storage Pool (0.0.40)

Virtual-machine storage is a third managed pool role alongside video and personal files.

```text
storage.vm_mounts=
storage.vm_policy=most_free
storage.vm_reserve_percent=10
storage.vm_reserve_gb=0
storage.vm_pinned_mount=
```

A filesystem may belong to several Home AI roles at the same time, for example
`video+personal+vm`. VM placement uses only online, writable, capacity-readable
volumes assigned to the `vm` role. The pool reserve is applied before checking that
the requested virtual-disk size fits.

`POST /api/hypervisor/storage/preview` accepts only `size_gib`. It does not accept
a host path or filename. A successful preview identifies the selected filesystem by
stable filesystem UUID and does not create a disk image.
