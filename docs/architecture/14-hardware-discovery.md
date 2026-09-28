# Hardware Discovery

Core v0.1 includes a read-only local hardware inventory.

## Sources

The initial implementation deliberately avoids shelling out to tools such as `lshw`, `lsblk`, `ip` or vendor GPU utilities.

It reads from:

- `/proc/cpuinfo`
- `/proc/meminfo`
- `/proc/uptime`
- `/sys/block`
- `/sys/class/net`
- `/sys/class/drm`
- Go's standard network-interface API

This keeps the base Core independent from optional command-line packages and reduces command parsing/security surface.

## Scope

Core discovery is descriptive, not authoritative configuration management.

It reports enough information to:

- display a useful node dashboard
- advertise basic node capabilities later
- detect the presence of storage/network/GPU resources
- support diagnostics

## Ownership boundaries

Core does **not**:

- partition or format disks
- mount filesystems
- configure interfaces
- install GPU drivers
- assign stable storage IDs
- allocate devices to workloads

Those operations belong to future modules and the privileged-operation boundary.

## Stable identity

Kernel names such as `sda`, `nvme0n1` and interface names are properties, not global resource identities.

Future Storage/Network modules will establish stable resource identifiers from appropriate provider data and persist those mappings in Core/module state.
