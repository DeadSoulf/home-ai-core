# Hypervisor Core

## Architecture

Home AI Core uses the Linux virtualization stack instead of implementing a hypervisor itself:

```text
Home AI Core Web/API
        |
Virtualization Core
        |
libvirt API
        |
QEMU
        |
KVM
        |
Linux kernel / hardware
```

libvirt remains the source of truth for virtual-machine definitions and runtime state. Home AI
Core adds the Web UI, permissions, audit, automation, storage policies and future backup logic.

The Core does not manage VM inventory by parsing `virsh` output and does not construct QEMU
command lines directly.

## 0.0.30 foundation

The first implementation is intentionally read-only.

Hypervisor Core detects:

- CPU virtualization flags (Intel VMX / AMD SVM)
- `/dev/kvm` presence
- whether the Home AI Core process can read/write `/dev/kvm`
- QEMU executable availability
- libvirt runtime-library availability
- read-only connection to `qemu:///system`

When libvirt is available, it reads host information:

- CPU model
- logical CPU count
- CPU frequency
- host RAM
- NUMA/socket/core/thread topology
- libvirt version
- QEMU/hypervisor version
- active connection URI

It also inventories all libvirt domains:

- name
- UUID
- state
- active/inactive state
- vCPU count
- current/max memory
- autostart
- accumulated CPU time

## Runtime dependency strategy

Home AI Core loads `libvirt.so.0` dynamically. This means the Core can still compile and start
on a machine where libvirt is not yet installed.

Missing KVM, QEMU or libvirt therefore degrades only the Hypervisor module; it does not stop
Security, Web, Cameras, Storage or the rest of Home AI Core.

A normal Debian KVM/libvirt host will eventually require packages similar to:

```text
qemu-system-x86
libvirt-daemon-system
libvirt-clients
```

The Web dashboard reports what is actually missing before VM management is enabled.

## Security

The current API is read-only:

```text
GET /api/hypervisor
```

It requires:

```text
hypervisor.view
```

No VM lifecycle or destructive operation is available in 0.0.30.

Future mutation endpoints will require `hypervisor.manage`, the standard Home AI request
header and audit entries.

## Planned sequence

1. Validate host capability and VM inventory.
2. VM lifecycle: start, graceful shutdown, reboot, force-off and autostart.
3. VM creation/editing with safe libvirt XML generation.
4. qcow2/raw disks, ISO attachment and Storage Pool integration.
5. NAT, bridge and isolated virtual networks.
6. browser console.
7. snapshots and rollback.
8. backup/restore separate from snapshots.
9. USB/PCI/VFIO passthrough.
10. disposable sandbox VMs for generated-code testing.
