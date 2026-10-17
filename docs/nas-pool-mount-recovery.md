# NAS folder creation: mount point mismatch

## Symptom

Creating a Files folder returns HTTP 502 `file_folder_prepare_failed` with:

```text
NAS pool root must be the filesystem mount point itself
```

This error originates in the privileged storage helper
(`cmd/home-ai-core-updater/nas_ops.go`), **not** in the Windows client,
username or logical folder name.

Home AI Core deliberately requires a NAS pool root to be a real writable
filesystem mount directly below `/mnt/home-ai-core/`, never an arbitrary
directory or the system root filesystem. This protects against an unplugged
disk silently redirecting writes to the server's system drive.

## Read-only diagnosis

In **Files → Storage**, find the existing pool's **saved root path**. Copy it
exactly, e.g. `/mnt/home-ai-core/sdb1`. In a trusted Linux administrator
terminal:

```sh
POOL_ROOT=/mnt/home-ai-core/sdb1  # replace with the exact displayed path
findmnt -T "$POOL_ROOT" -o TARGET,SOURCE,FSTYPE,OPTIONS
findmnt -M "$POOL_ROOT" -o TARGET,SOURCE,FSTYPE,OPTIONS
lsblk -o NAME,PATH,TYPE,FSTYPE,UUID,MOUNTPOINTS
```

`findmnt -T` tells you which filesystem **covers** the directory. It may
report `/` even if the path exists and the NAS disk is *not mounted*.
`findmnt -M` requires an exact mountpoint and should show the NAS device.
If the second command prints no exact mount, **do not retry folder creation**
until the original filesystem is restored.

Check which device/UUID belongs to the existing pool. If the drive is missing,
diagnose hardware, LVM/partition activation and filesystem errors first.
If it is mounted elsewhere, reconcile the original mountpoint with an
administrator-approved procedure; do not create another pool pointing at an
arbitrary path.

The privileged helper may have a different systemd mount namespace than the
host. If the host does show the correct exact mount but the error persists,
compare **read-only** views:

```sh
HELPER_PID="$(systemctl show -p MainPID --value home-ai-core-updater)"
sudo nsenter --mount=/proc/1/ns/mnt -- findmnt -T "$POOL_ROOT" -o TARGET,SOURCE
sudo nsenter --mount="/proc/$HELPER_PID/ns/mnt" -- findmnt -T "$POOL_ROOT" -o TARGET,SOURCE
```

If these differ, **do not force a mount or folder write from the helper's
stale namespace**. Check mount propagation and the unit configuration; after
verifying the disk, plan a helper-service restart in an appropriate
maintenance window. A helper restart can interrupt in-flight storage jobs.

## Safe recovery

1. Verify the source device and filesystem UUID against the pool's saved
   `storage_device` and `storage_filesystem_uuid` (from Files storage
   metadata or the system's disk inventory).
2. Restore the **same** storage filesystem at the **exact saved path**, using
   the existing systemd/fstab mount configuration or a reviewed administrative
   mount operation. If using the Files UI Mount action, ensure it uses the
   existing pool root and original device; a different default path will not
   repair that pool.
3. Confirm `findmnt -M "$POOL_ROOT"` shows the expected device, the mount is
   writable, and the Core service can traverse it. Only then retry.
4. Configure persistent mounting and predictable UUID-based identity so the
   mount survives reboot. Check service startup dependencies as appropriate.

Do **not** format or repartition the drive, delete/recreate the pool, move
`.home-ai`, disable the exact-mountpoint validation, run `chmod -R 777`
or automatically redirect writes onto the system filesystem.

The helper now reports the saved root and the covering mount when they
disagree. If the host sees the root mount but the helper cannot see it,
the error identifies the separate namespace condition explicitly.
