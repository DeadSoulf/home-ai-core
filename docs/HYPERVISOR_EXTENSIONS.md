# Hypervisor extension contracts

Version 0.0.34 introduced lifecycle, 0.0.38 added a non-mutating
`create_preview` contract, and 0.0.39 adds persistent domain definition.
`HypervisorManager::capabilities()` is the implementation registry while
`runtimeCapabilities()` masks backend-dependent features for GET inventory.
`create_preview` is always true; `create` is exposed only when its libvirt
symbol group is available. `edit`, `delete`, `snapshots`, `disks`,
`networks` and `console` remain false. Unknown lifecycle action names fail
closed. Clients must treat absent capability names as false for compatibility.
Do not flip a flag until its backend, authorization, validation and integration
tests ship together. Existing inventory field names and the Web UI route stay stable.

## Common boundary

- Resource identity: libvirt UUID; display names are untrusted text.
- Queries use read-only connections. Mutations use short-lived writable connections
  and the existing result envelope (`success`, `code`, `message`, observed `state`).
- Every mutation needs `hypervisor.manage`, the request header, audit outcome and
  explicit confirmation bound to the resource. More destructive resource operations
  should introduce narrower permissions before enabling them.
- Preserve Core startup without libvirt. Load each new feature's symbols as its own
  optional group, so missing snapshot/console support does not disable lifecycle.
- Implement long operations as durable jobs with IDs, bounded concurrency and
  cancellation semantics; the current 30-second duplicate guard is not that queue.
- Use domain XML revision checks in addition to state checks for configuration edits.
  Serialize Core operations per domain and propagate conflicts from external tools.

## Resource modules to implement next

| Module | Input and validation boundary | Backend and completion contract |
| --- | --- | --- |
| Create / Edit | Typed VM definition: name, UUID, architecture, machine type, vCPU count, memory bytes, boot, disk IDs, NIC IDs. 0.0.38 implements server-generated preview; 0.0.39 enables persistent creation for name/vCPU/memory/architecture/machine type only. Raw XML from the UI remains forbidden. Edit preview still requires configuration revision and live-vs-next-boot scope. | 0.0.39 validates host CPU/RAM and duplicate names, defines with `virDomainDefineXML`, then reads name/UUID/state back before success. It creates no storage/network resources and never starts the VM automatically. Storage creation/rollback and edit remain future work. |
| Delete | UUID, revision, inactive state, explicit confirmation. Preserve disks by default; deleting volumes requires a separate ownership/reference check and confirmation. | Undefine domain configuration only. Handle managed saves, snapshots and NVRAM deliberately; never reuse Force Off as Delete. |
| Disks / ISO | Storage Pool volume ID, format (qcow2/raw), size and attachment target. Validate quotas, free space, ownership, references and ISO read-only status. Do not accept arbitrary host paths. | Volume operations plus libvirt attachment APIs; report live and persistent configuration independently. Never remove shared volumes. |
| Networks | Network UUID, NAT/isolated/bridge mode, subnet, DHCP range and bridge ID. Validate conflicts and bridge membership. | libvirt network definitions; coordinate host bridge changes with the existing Network module and its rollback policy. |
| Snapshots | Domain UUID, snapshot ID, configuration revision, disk eligibility and quiesce policy. Explicitly distinguish internal/external and memory/disk snapshots. | Discover driver capabilities; asynchronous jobs for create/revert/delete. Do not present snapshots as backups; never silently fall back to a crash-consistent snapshot. |
| Console | Domain UUID and console type discovered server-side. Never expose raw VNC/SPICE ports or passwords in inventory. | Authenticated same-origin proxy with short-lived tickets, permission revalidation and session revocation. No arbitrary upstream URL supplied by the browser. |

Each module should add manager tests against an isolated backend, HTTP tests for
authorization/confirmation/conflicts, and browser tests for disabled capabilities,
pending results and errors before becoming available in the existing VM cards.
