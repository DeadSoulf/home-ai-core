# Home-AI Development Roadmap

This roadmap follows the canonical product direction in `../PRODUCT_VISION.md`.

Earlier foundation phases produced useful Core capabilities. Generic container/virtualization management is no longer on the critical path.

## F0 — Core Foundation — mostly complete

Implemented foundation:

- Core daemon/API;
- node identity;
- SQLite state + migrations;
- authentication/sessions;
- RBAC/CSRF/audit;
- jobs;
- durable events;
- WebSocket realtime;
- Module SDK/registry/dependency planning;
- signed module repository foundation;
- Web UI shell;
- hardware/system discovery;
- low-level storage;
- privileged helper;
- Web update system + rollback capability;
- CI/release pipelines.

### Remaining F0 hardening

- split oversized privileged updater/storage helper implementation;
- live rollback acceptance test;
- extend permission model toward resource scopes;
- stable signed release-channel design;
- failure testing.

## F1 — File Storage / NAS

Build the first major user-facing product domain on the existing storage foundation.

- pools/volumes;
- private per-user folders;
- shared folders;
- Web file manager;
- SMB;
- NFS where required;
- quotas/policies;
- snapshots/backup capabilities where supported;
- storage health/capacity UX.

## F2 — Native Smart Home

Home-AI owns its smart-home data model and automation engine.

- device/entity/state model;
- rooms/zones;
- discovery and pairing;
- Zigbee module;
- Matter/Thread module;
- MQTT module;
- Wi-Fi/LAN integrations;
- Bluetooth integrations;
- Modbus;
- scenes;
- schedules;
- automation/rule engine;
- notification/event integration;
- resource-scoped permissions and audit.

A future compatibility bridge to other systems is optional; Home Assistant is not the platform foundation.

## F3 — Cameras / NVR

- camera entities;
- RTSP ingest;
- ONVIF where useful;
- live view;
- recording;
- timeline/archive;
- configurable recording storage allocation;
- oldest-first overwrite/retention;
- protected recordings;
- motion/object events;
- AI vision integration points;
- known-person database and face recognition when hardware permits.

## F4 — Local AI Runtime and Agent

### Runtime

- local LLM adapters;
- multimodal/vision adapters;
- GPU/accelerator placement hints;
- model lifecycle/storage;
- resource limits.

### Agent

- tool registry;
- permission-derived tool access;
- approval workflow;
- user/home context;
- memory;
- file search;
- NVR search;
- Smart Home control;
- server diagnostics;
- automation proposals;
- self-improvement proposals;
- versioned/audited/reversible AI-driven changes;
- optional external AI providers.

## F5 — Voice

- microphone terminal identity;
- wake word;
- STT;
- user/context resolution;
- AI Agent request;
- permission/approval;
- TTS.

## F6 — Remote Access and Client File Transfer

### WireGuard

- owner-controlled secure tunnel;
- device enrollment and revocation;
- service exposure policy.

### Windows client

- copy selected files to allowed Home-AI folders;
- resumable upload;
- integrity verification;
- LAN/remote operation;
- scheduling/automatic copy when enabled.

Android follows after the server API is stable.

## F7 — Multi-node Foundation

- secure node enrollment;
- authenticated/encrypted trust;
- heartbeats;
- node/resource inventory;
- CPU/RAM/GPU/storage capability advertisement;
- workload requirement model;
- storage/locality metadata;
- degraded operation.

## F8 — Distributed Scheduling and Failover

- scheduler;
- placement of movable workloads;
- AI/video processing distribution;
- capacity accounting;
- affinity/locality;
- node-bound resource handling;
- reassignment after failure where possible;
- cluster-wide Web view.

Cluster leader/control-state architecture is intentionally deferred until requirements are validated; current APIs must not force a permanent single-master model.

## F9 — Production / Commercial Hardening

- stable/dev channels;
- signed release manifests/artifacts;
- permission/security review;
- backup/recovery validation;
- multi-node failure tests;
- NVR/storage long-duration tests;
- AI approval/audit tests;
- licensing/provenance audit;
- documentation;
- installer/recovery UX.

## Parked optional capabilities

These may be implemented later if there is a real Home-AI use case:

- generic container management;
- generic third-party app marketplace;
- KVM/VM management;
- iOS client.

They must not block NAS, Smart Home, NVR, AI, Voice or Cluster development.
