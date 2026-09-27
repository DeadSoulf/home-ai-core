# Home AI Core — Master Plan

This document is the single high-level source of truth for project scope, readiness, dependencies,
acceptance state, and implementation order. Module-specific documents remain authoritative for
low-level details.

## Current baseline

- Branch: `develop`
- Version: `0.0.49`
- Current focus: finish the platform foundation before resuming major Hypervisor/Storage expansion.
- CI: Debian build/test workflow is active and the current `develop` snapshot passes it.
- Virtualization test environment note: the current development server may itself run inside a KVM
  guest without VMX/SVM passthrough. Absence of nested `/dev/kvm` in that environment is therefore
  a host limitation, not by itself a Hypervisor Core defect.

## Status model

Every module is tracked using the same fields:

- **Planned** — intended project scope.
- **Implemented** — functionality present in source code.
- **Verified** — covered by CI and/or real server acceptance.
- **In progress** — work currently being closed.
- **Next** — next concrete deliverable.
- **Readiness** — engineering estimate, not a release guarantee.
- **Dependencies** — modules that must remain stable underneath it.

A feature is not considered complete only because an API, UI page, or class skeleton exists.
Completion requires implementation, safe failure behavior, tests, documentation, and acceptance
on a suitable environment when hardware integration is involved.

## Module matrix

| Module | Readiness | Implemented / verified today | In progress / next | Dependencies |
|---|---:|---|---|---|
| Core Runtime / Module Manager | ~93% | Runtime, Config, Logger, Event Bus, global event subscriptions, in-memory Notification Center, module dependency ordering, lifecycle, health, reverse shutdown, dependency-safe restart, bounded unhealthy-only runtime watchdog, module API/UI | Real-server acceptance, explicit Web/API restart control, config reload, resource limits, signed external modules | Security, Event Bus |
| System / Server Admin | ~88% | CPU/RAM/load/uptime/filesystem, health, GPU inventory, systemd integration, host readiness diagnostics for toolchain/helpers/media/TLS/KVM/QEMU/libvirt | Real-server acceptance, processes/services, journal, sensors, power actions, config backup | Core, Security |
| Update Core | ~85% | Git update, clean-tree guard, FF-only pull, CMake/Ninja, CTest, rollback, restart | Signed releases/modules, production channels, cluster update, downgrade/data migrations | Core, Security |
| Web UI / API | ~84% | Authenticated responsive RU/EN UI, permission-aware routes/navigation, subsystem pages, protected notification feed and host readiness UI | Unified API errors, SSE/WebSocket realtime transport, API docs | Core, Security |
| Security Core | ~82% | SQLite identity store, PBKDF2, roles/overrides, persistent sessions, audit, HTTPS/TLS, session-bound CSRF, source-aware login throttling, protected config forms | MFA, encrypted secrets store, trusted reverse-proxy mode, signed update/module verification, audit retention/export | Core |
| Network / WireGuard | ~70% | Interface inventory, DHCP/static IPv4, gateway/DNS, NetworkManager/networkd/ifupdown, guarded helper, rollback, WireGuard profile management | Bridge/VLAN/bonding, advanced routes, DNS diagnostics, firewall, Hypervisor networks | Core, Security |
| Camera Core | ~65% | RTSP registry, encrypted credentials, ffprobe metadata, snapshots, ONVIF discovery/profiles/stream URI/PTZ, SADP/LAN discovery, Live JPEG | Continuous video transport, recorder integration, events | Core, Security, Network |
| Storage Core | ~60% | Discovery, mount/unmount, EXT4, UUID, pools, reserves, policies, VM pool, disk placement preview | SMART/temp/wear, partition wizard, quotas, retention, RAID awareness, backup/ISO/model pools | Core, Security |
| Hypervisor Core | ~45% | KVM/QEMU/libvirt detection, VM inventory/lifecycle, pause/resume, autostart, VM definition/create/read-back, managed storage placement preview | Real virtual disks, ISO, networking, editor, console, snapshots, backup, passthrough, migration | Core, Security, Storage, Network |
| Cluster Core | ~30% | Controller/Worker roles, heartbeat, node health, placement scheduler | Enrollment/mTLS, workload assignment, failover, drain/maintenance, shared state/storage awareness | Core, Security, Network |
| NVR / Recorder | ~15% | Camera input infrastructure exists | Continuous ingest/recording, pre-buffer, archive, timeline, playback, retention, analytics events | Camera, Storage |
| Personal Data / Files | ~10% | Permission and storage-role foundation | Files API, indexing/search, versions, recycle bin, backup, controlled AI access | Storage, Security |
| Device Core / Smart Home | ~5% | UI/permissions/architecture slots | Unified device model, MQTT/HTTP/TCP/UDP/ONVIF/Modbus/Zigbee/BLE/GPIO drivers | Core, Security, Network |
| Automation Core | ~5% | UI/permissions/architecture slots | Rule engine, triggers, conditions, actions, scenarios, schedules, retries/history | Event Bus, Devices, Security |
| AI Brain | ~5% | GPU detection/assignment, UI/permissions foundation | Local inference runtime, model manager, controlled tools, planner | Stable platform modules |
| Memory Core | ~2% | Runtime/architecture layout | Semantic, episodic and experience memory | AI Brain, Storage |
| Backup Core | ~5% | Architectural scope only | Core/database/files/VM/NVR/automation/memory/cluster backup orchestration | Storage, Security |

## Phase A — close the platform foundation

This phase must be completed before large new service layers are treated as production-capable. The code/CI portion is complete through 0.0.49; real Debian-host acceptance remains.

| Item | State | Acceptance |
|---|---|---|
| 0.0.40 regression baseline | IMPLEMENTED / CI-VERIFIED LATER | Current CI builds/tests code that includes the 0.0.40 Hypervisor storage work. Full hardware KVM acceptance requires a host with virtualization exposed. |
| HTTPS/TLS | DONE in 0.0.41 | TLS startup validation, TLS 1.2+, HSTS, Secure session cookie path, tests |
| Session-bound CSRF | DONE in 0.0.42 | Mutation API requires request marker + session CSRF token; cross-session token rejected |
| Source-aware login throttling | DONE in 0.0.43 | Independent username and peer-source throttling; forwarded headers are not trusted by default |
| Configuration-form CSRF compatibility | DONE in 0.0.44 | Legacy settings/config forms follow the same protected mutation path |
| Documentation consolidation | DONE for current foundation | MASTER_PLAN is the high-level source of truth and ROADMAP/subsystem docs are synchronized with the current foundation milestones |
| Module restart + watchdog/recovery | DONE in 0.0.46 (CI verified) | Dependency-aware restart, unhealthy-only watchdog, bounded retries/cooldown; real-server acceptance remains |
| Unified events / notifications | DONE in 0.0.47–0.0.48 (CI verified) | Event Bus global subscriptions, bounded in-memory Notification Center, protected API and System-page feed; persistence/retention remains a later enhancement |
| Host requirements diagnostics | DONE in 0.0.49 (CI verified) | Protected readiness API/UI checks update toolchain, storage/network helpers, media tools, HTTPS, KVM/nested virtualization and QEMU/libvirt with actionable messages |

### Phase A exit gate

The platform foundation can be treated as approximately 90–95% complete when:

1. current `develop` passes build and the full acceptance suite;
2. Security hardening above remains enforced on all mutation paths;
3. a degraded module can be recovered or cleanly marked failed without taking down unrelated modules;
4. host requirement failures are explicit and actionable;
5. the documentation set matches actual runtime behavior.

## Phase B — Hypervisor to a complete VM workflow

The old provisional version numbers for these items are no longer authoritative because
`0.0.41–0.0.44` were used for Security hardening. Preserve the functional order below and assign
new version numbers when implementation starts.

1. **Virtual Disk**
   - server-generated image name;
   - managed Storage Pool selection only;
   - `qcow2` first, `raw` where justified;
   - free-space/reserve validation;
   - constrained privileged helper;
   - correct QEMU/libvirt ownership;
   - rollback on any partial failure;
   - attach to persistent VM;
   - libvirt read-back verification;
   - never accept arbitrary host paths from Web UI.

2. **ISO / CD-ROM**
   - managed ISO pool and inventory;
   - attach/detach;
   - boot ISO;
   - no arbitrary host path.

3. **Virtual Networking**
   - NAT, bridge and isolated networks;
   - interface/network selection;
   - MAC generation;
   - NIC attach/read-back.

4. **VM Editor**
   - controlled updates of CPU, RAM, disks, NICs, boot order and autostart.

5. **Console**
   - serial console first;
   - browser VNC/SPICE proxy after the safe transport/auth model is defined.

6. **Snapshots**
   - create/list/rollback/delete with strict state validation and confirmation.

7. **Backup / Restore**

8. **Hardware passthrough**
   - PCI/GPU;
   - USB.

9. **Cluster integration**
   - Controller requests workload placement;
   - scheduler chooses a suitable Worker;
   - VM state is read back into cluster inventory.

## Phase C — Storage completion

Complete the shared storage abstraction used by Hypervisor, NVR, Files, Backup and AI:

- SMART, temperature and health;
- HDD/SSD/NVMe classification and wear;
- GPT/partition workflow;
- filesystem recognition and safe eject;
- quotas and retention;
- RAID/mirror awareness and degraded events;
- Backup, ISO and AI/model pools.

## Phase D — NVR / Video

Functional order:

`continuous RTSP ingest -> recorder -> browser live transport -> continuous recording -> pre-event buffer -> event recording -> archive DB -> timeline -> playback -> retention -> multi-disk placement -> motion -> object analytics -> Automation events`.

## Phase E — Personal Files

`Files API -> upload/download -> directories -> permissions -> metadata -> indexing -> search -> versions -> recycle bin -> backup -> controlled AI read access`.

## Phase F — Device Core

Canonical model:

`Device -> capabilities -> state -> commands -> events`

Driver order is determined by real deployments, with the architecture prepared for MQTT, HTTP,
TCP/UDP, ONVIF, Modbus, Zigbee, BLE and GPIO.

## Phase G — Automation

Canonical execution model:

`Event -> Trigger -> Conditions -> Actions`

Then add rules, scenarios, schedules, timers, device/camera/system/storage/VM events, execution
history, retries and permission boundaries.

## Phase H — real Cluster workloads

Connect the existing scheduler to actual workloads rather than rewriting the scheduler:

- Recorder placement;
- VM placement;
- AI jobs;
- Automation/service placement;
- failover;
- node draining and maintenance;
- shared state and storage awareness;
- cluster upgrade.

## Phase I — AI Brain and Memory

AI remains above the platform, not underneath it. Core services must remain functional if AI is
disabled or fails.

Order:

1. local inference runtime and model manager;
2. permission-controlled tools for System/Storage/Cameras/Devices/Automation/Files/Hypervisor;
3. planner;
4. semantic + episodic + experience memory;
5. feedback/outcome tracking;
6. only much later: controlled self-development through
   `proposal -> sandbox -> build -> test -> review/policy -> deployment -> rollback`.

The AI runtime must never receive unrestricted shell/root access as its normal control interface.

## Project dependency map

```text
Core
 ├── Security
 ├── Event Bus
 ├── Module Manager
 └── Web/API
      ├── System
      ├── Network
      ├── Storage
      │    ├── Files
      │    ├── NVR
      │    ├── Hypervisor
      │    └── Backup
      ├── Cameras
      ├── Devices
      │    └── Automation
      ├── Hypervisor
      └── Cluster
           └── distributed workloads

AI Brain
 ├── Memory
 ├── Planner
 └── controlled tools
      ├── Files
      ├── Cameras
      ├── Devices
      ├── Automation
      ├── Hypervisor
      └── System
```

## Immediate execution order

1. Run the complete Phase A acceptance suite for 0.0.49 on the real Debian server with
   `bash scripts/acceptance-phase-a.sh /srv/home-ai-core`. For authenticated read-only API
   verification, set `HOMEAI_ACCEPTANCE_USER=<username>`; the script prompts for the password
   without placing it in command history.
2. Fix any host-only acceptance findings without expanding scope.
3. Resume Hypervisor with real managed virtual disk creation.
4. Continue ISO -> networking -> VM editor -> console -> snapshots/backup.
5. Close Storage health.
6. Build Recorder/NVR.

Update this file after every development snapshot so project state does not have to be reconstructed
from commit history.


## Phase A real-host acceptance command

The repository includes a non-destructive acceptance runner:

```bash
cd /srv/home-ai-core
bash scripts/acceptance-phase-a.sh /srv/home-ai-core
```

To include authenticated read-only checks for session, modules, notifications and readiness:

```bash
cd /srv/home-ai-core
HOMEAI_ACCEPTANCE_USER=admin bash scripts/acceptance-phase-a.sh /srv/home-ai-core
```

The password is read interactively and is not written to command history. The acceptance runner
does not format disks, change network configuration, mutate VMs or call privileged management
actions. It performs a clean out-of-tree build, the full CTest suite, systemd/service checks,
helper/tool inventory, Web security-header checks and host virtualization checks.

For the current KVM-based development VM, missing VMX/SVM passthrough and missing `/dev/kvm`
are reported as warnings because nested virtualization is a host limitation already identified.
On a non-virtualized physical acceptance host, those conditions are failures. TLS disabled on the
acceptance host is a failure: Phase A is not closed until the real management interface is
actually exercised over HTTPS.
