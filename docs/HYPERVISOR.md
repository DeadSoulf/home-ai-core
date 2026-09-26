# Hypervisor Core

## Debian setup (0.0.35)

From the updated repository on the Debian server, run:

```bash
sudo bash scripts/setup-hypervisor.sh check
sudo bash scripts/setup-hypervisor.sh install
```

`check` only inspects KVM, QEMU and a libvirt connection. When run with sudo, it
tests access as the configured non-root `home-ai-core.service` user; without sudo,
it tests the current account. A connection check does not prove permission for
every domain operation. The script does not create, start or stop VMs directly.

`install` requires Debian and an existing non-root Core service account. It installs
QEMU/libvirt packages, adds that account to `libvirt` and `kvm`, enables local libvirt
daemons while preserving an existing modular setup, and restarts Home AI Core to load
the library and new group membership. These groups grant powerful host virtualization
access. Existing libvirt domain autostart policies apply when daemons start.
No network listener or passwordless sudo rule is added. Errors stop installation;
package and group changes already completed are retained, so inspect the output before
retrying. No automatic rollback or daemon architecture migration is attempted.

If Debian itself runs inside Proxmox, `/dev/kvm` may require nested virtualization
enabled in the parent host/VM configuration. Installing packages alone cannot enable
that hardware capability. The Web UI distinguishes missing libvirt, unavailable
connections and missing KVM/QEMU and provides the appropriate setup/check command.

The daemon selection follows the [libvirt daemon architecture](https://libvirt.org/daemons.html).

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

## 0.0.34–0.0.37 lifecycle

The existing inventory remains read-only. Lifecycle requests open a separate writable
`qemu:///system` connection only after Web/API authorization and explicit confirmation.
The service account needs libvirt write access (socket policy / polkit); a successful
inventory read does not imply that writes are permitted. Do not run the whole Core as root.

`POST /api/hypervisor/action` accepts URL-encoded fields:

```text
uuid=<canonical lowercase UUID>
action=start|shutdown|reboot|pause|resume|autostart-on|autostart-off|force-off
expected_state=<state shown when confirming>
confirmation=<same UUID>
```

Every request requires an authenticated session, `hypervisor.manage` and
`X-HomeAI-Request: 1`. The UI asks for confirmation for all commands; Force Off also
requires typing the UUID and warns about data loss. The API independently validates
the confirmation target and rereads the domain state before dispatch. Domain names
are display-only and never become executable commands or identifiers for mutation.

| Observed state | Actions |
| --- | --- |
| shutoff | Start, Autostart on/off |
| running, blocked | Shutdown, Reboot, Pause, Force Off, Autostart on/off |
| paused | Resume, Force Off, Autostart on/off |
| shutdown, crashed, suspended | Force Off, Autostart on/off |
| unknown, no-state, unreadable | Autostart on/off only when the domain state is readable |

Commands use `virDomainCreate`, `virDomainShutdown`, `virDomainReboot`,
`virDomainSuspend`, `virDomainResume`, `virDomainSetAutostart` and
`virDomainDestroy`. Pause/resume and autostart are loaded as optional lifecycle
extensions so an older libvirt runtime can still keep the original lifecycle controls.
Force Off does not undefine a persistent VM or delete its disks.
Transient libvirt domains can disappear when stopped, as defined by libvirt.

Success returns HTTP 202 with `{success, code: "accepted", message, state}`.
`state` is an immediate observation, not proof of completion. In particular, guests
may ignore Shutdown/Reboot; the UI polls every 10 seconds and keeps the command
result separate from inventory status. No automatic escalation to Force Off occurs.
Graceful requests suppress duplicate non-force operations for 30 seconds per UUID,
or until a powered-off state is observed. `pending_action` exposes this guard in the
inventory; an unreadable VM reports `error` and has no enabled actions.
This guard is process-local, not a durable job queue; restarting Core clears it.

Errors return `success: false`, `code`, `message`, `state`:

- 400: invalid UUID/action, missing confirmation or expected state.
- 403: insufficient Core or libvirt permissions, missing request header.
- 404: domain disappeared.
- 409: stale/disallowed state or recent pending operation.
- 502: libvirt operation/read failure.
- 503: module, lifecycle symbols or writable connection unavailable.

Authorized action attempts are audited with actor, UUID, action and result code.
No shell commands, credentials or raw form bodies enter the audit record.
External administrators can still change state between read and dispatch; libvirt
remains authoritative and resulting errors are returned, never retried automatically.

`GET /api/hypervisor` retains its existing fields and adds `allowed_actions` per VM
(empty for view-only users) and `capabilities`. Capability flags describe implemented
features, not host authorization; `allowed_actions` is advisory and POST revalidates.
Missing lifecycle symbols leave read-only inventory functional.

See [extension contracts](HYPERVISOR_EXTENSIONS.md) for the next module boundaries.

## Validation

Build and run `ctest --test-dir build --output-on-failure` on Linux. Lifecycle and
HTTP integration tests use a dedicated fake `libvirt.so.0` selected only for those
tests, covering denied writes, missing domains, rejected operations, ignored guest
shutdown, state conflicts, duplicate suppression and resource cleanup. These tests
never operate on host VMs. The ordinary manager test covers optional/missing libvirt.

After CTest generates UI fixtures, run:

```text
node tests/test_hypervisor_ui.cjs [path-to-playwright] [browser-channel]
node tests/test_localization.cjs [path-to-playwright]
```

Real KVM/guest-agent and Debian service-policy verification remains a deployment
check: use a disposable VM, verify each command and denied permissions, confirm
the audit trail, and verify other pages before restarting the normal service.

## 0.0.30 foundation (historical)

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

Missing KVM, QEMU or libvirt does not stop Security, Web, Cameras, Storage or the rest of
Home AI Core. A completely missing libvirt runtime is treated as an unconfigured optional
capability rather than a module fault: Hypervisor Core remains healthy in detection-only mode.
If libvirt is installed but Home AI Core cannot connect to `qemu:///system`, the module is
reported as degraded because that indicates a daemon, socket or permission problem that needs
attention.

A normal Debian KVM/libvirt host will eventually require packages similar to:

```text
qemu-system-x86
libvirt-daemon-system
libvirt-clients
```

The Web dashboard reports what is actually missing before VM management is enabled.

## Security

The original inventory API is read-only:

```text
GET /api/hypervisor
```

It requires:

```text
hypervisor.view
```

No VM lifecycle or destructive operation is available in 0.0.30.

The 0.0.34 lifecycle endpoint above adds `hypervisor.manage`, the standard Home AI
request header and audit entries.

## Planned sequence

1. Validate host capability and VM inventory.
2. VM lifecycle: start, graceful shutdown, reboot, pause/resume, force-off and autostart. (Implemented through 0.0.37)
3. VM creation/editing with safe libvirt XML generation.
4. qcow2/raw disks, ISO attachment and Storage Pool integration.
5. NAT, bridge and isolated virtual networks.
6. browser console.
7. snapshots and rollback.
8. backup/restore separate from snapshots.
9. USB/PCI/VFIO passthrough.
10. disposable sandbox VMs for generated-code testing.
