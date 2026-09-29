# Home-AI-Core — Current State Audit

**Audit baseline:** `0.1.58-dev`  
**Date:** 2026-09-29

This document answers three questions:

1. What has already been built?
2. Which parts directly support the final Home-AI product?
3. Which earlier ideas should be deprioritized or removed from the main roadmap?

## Executive conclusion

The current implementation is **mostly useful foundation work**.

There is no evidence that a large implemented subsystem must be deleted simply because the final product vision became clearer.

The main excess was **roadmap scope**, not working code: earlier plans elevated generic container management, virtualization and third-party self-hosted applications too close to the center of the product.

Those capabilities may remain optional later, but the primary roadmap is now:

```text
Core Foundation
  -> NAS / Files
  -> Smart Home
  -> Cameras / NVR
  -> AI Agent
  -> Voice / Remote clients
  -> Multi-node Cluster
```

## Keep — directly useful foundation

| Existing component | Decision | Why it is needed |
|---|---|---|
| Core REST API | KEEP | Common control plane for Web, voice, clients, modules, AI and cluster |
| SQLite state + migrations | KEEP | Local durable control metadata and safe upgrades |
| Stable node identity | KEEP | Direct foundation for future cluster membership/resource ownership |
| Authentication / sessions | KEEP | Multi-user Home-AI requires identity |
| RBAC / permissions | KEEP + EXTEND | Needed for users, rooms, devices, files, cameras and AI tool access |
| CSRF and Web security | KEEP | Required for privileged local administration |
| Audit log | KEEP | Essential for AI actions, security, storage and automation changes |
| Persistent Jobs | KEEP | Needed for file operations, NVR maintenance, model downloads, backups and distributed workloads |
| Durable Events | KEEP | Needed for smart-home state, job events, camera events and cluster control events |
| WebSocket realtime | KEEP | Needed for live UI updates, device state and task progress |
| Module SDK / registry | KEEP | Final product is explicitly modular |
| Dependency planner | KEEP | Required for safe module installation/upgrades |
| Signed module repository foundation | KEEP | Strong fit for modular/commercial distribution |
| System information | KEEP | Needed for administration and future cluster resource inventory |
| Hardware discovery | KEEP | Needed for GPUs, camera accelerators, storage and protocol adapters |
| Low-level Storage | KEEP | Foundation of future NAS and NVR archive storage |
| Privileged helper boundary | KEEP | Correct security boundary for host/storage/network operations |
| Update System v2 | KEEP | Successfully tested Web-driven lifecycle; critical product infrastructure |
| Rollback support | KEEP / TEST | Needed for reliable commercial/product updates |
| Web UI shell | KEEP | Current main user interface |
| CI / release workflows | KEEP | Needed for safe evolution and commercial-quality releases |
| Third-party license policy | KEEP | Important because commercial distribution is a goal |

## Keep, but change its future role

### Module Store

The signed repository and manifest work is useful.

It should become the distribution mechanism for **Home-AI modules** such as:

- Smart Home protocol modules;
- NVR;
- AI runtimes;
- Voice;
- WireGuard;
- NAS services;
- Cluster extensions.

It should not primarily become a generic marketplace for unrelated self-hosted applications.

### Jobs and Events

These are important, but camera frames/high-frequency telemetry must not be pushed through the durable event log.

Use the event system for domain/state events and references:

- motion detected;
- person detected;
- recording created;
- device state changed;
- workload moved;
- job completed.

Media payloads and high-rate telemetry need dedicated storage/streaming paths.

### SQLite

SQLite is correct for the current single-node Core.

It must not be assumed to become the final distributed cluster database.

Future cluster design may keep local SQLite for node-local state while adding a separate replicated/consensus layer for cluster-critical metadata.

### Privileged helper

The security boundary is correct.

The implementation is currently too concentrated in `cmd/home-ai-core-updater/main.go`, which mixes update and storage operations.

Keep the boundary; refactor the implementation.

## Deprioritize — not part of the main product path

### Generic Docker management

Container technology may be useful internally for module isolation or optional apps.

A Portainer-like general Docker management product is not a core Home-AI objective.

**Decision:** park as optional/future capability.

### Full virtualization / KVM management

VM support may be useful for advanced installations, but it does not unlock the main Home-AI product.

**Decision:** remove from the critical roadmap and revisit only after the primary product domains work.

### Generic third-party App Store

Earlier architecture examples included unrelated services such as media/collaboration/automation applications.

A generic self-hosting marketplace can create large scope without advancing autonomous-home functionality.

**Decision:** Home-AI module distribution first. Generic app marketplace only if there is a clear later product need.

### Home Assistant as Smart Home foundation

This conflicts with the clarified goal.

Home-AI will have its own device/entity/automation model.

**Decision:** remove Home Assistant from the primary architecture. A future optional compatibility bridge could exist, but Home-AI must not depend on it.

### iOS as an early client target

The clarified immediate client need is a Windows file-copy client. Android comes later.

**Decision:** no iOS priority in the current roadmap.

### Full Windows image backup

Not requested.

**Decision:** Windows client copies/synchronizes selected files; it is not a disk-image backup product.

## Missing major product domains

These are not failures of the current foundation. They are the next product layers.

### NAS / Files

Still required:

- storage pools;
- private user folders;
- shared folders;
- file manager;
- SMB/NFS;
- quotas/policies;
- snapshots/backup strategy.

### Smart Home

Still required:

- device/entity/state model;
- protocol adapters;
- pairing/discovery;
- event bus integration;
- scenes/rules/automations;
- room/zone permissions.

### Cameras / NVR

Still required:

- camera model;
- RTSP/ONVIF ingest;
- live view;
- recording/archive;
- ring retention;
- event timeline;
- AI vision hooks;
- face/person database.

### AI Agent

Still required:

- local model adapters;
- tool registry;
- permission-derived tool access;
- approval workflow;
- memory/context;
- automation proposal flow;
- self-improvement proposal/version/rollback mechanism;
- optional external-AI adapters.

### Voice

Still required:

- voice device identity;
- wake word;
- STT/TTS;
- user identification/authorization strategy;
- agent integration.

### Remote access

Still required:

- WireGuard module;
- device enrollment;
- revocation;
- remote routing/service policy.

### Windows file client

Still required:

- pairing;
- file selection;
- resumable transfer;
- integrity verification;
- user-folder authorization.

### Cluster

Still required:

- enrollment/trust;
- node heartbeats;
- resource advertisement;
- workload requirements;
- scheduler;
- locality rules;
- degraded mode/failover;
- leadership/control-state design.

## Current technical debt

1. Split `cmd/home-ai-core-updater/main.go` into routing, update and storage implementation packages.
2. Perform a real rollback acceptance test after the successful `0.1.58-dev` Web update.
3. Extend permission concepts from generic capabilities toward resource/zone/folder/camera scopes.
4. Define event-retention boundaries before Smart Home and NVR create high event volume.
5. Keep cluster-critical state abstract enough that single-node SQLite assumptions do not leak into future APIs.
6. Define a signed stable release channel before commercial/stable deployment.

## Immediate recommendation

Do **not** delete Jobs, Events, Modules, Module Store, Security, Storage or hardware inventory.

They are strategic assets for the final product.

The next engineering sequence should be:

1. finish/refactor the Core foundation;
2. build NAS/File Storage;
3. build the native Smart Home domain;
4. build NVR/Cameras;
5. build the local AI Agent on top of those domain APIs;
6. add Voice/remote clients;
7. add cluster scheduling/failover.
