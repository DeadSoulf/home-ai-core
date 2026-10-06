# Home-AI — Product Vision

**Status:** canonical product direction  
**Updated:** 2026-10-01  
**Current implementation baseline:** Home-AI-Core `0.1.114-dev`; AI Agent Foundation implementation started for `0.1.115-dev`

## 1. Mission

Home-AI is an **autonomous local platform for the entire home**.

A user should be able to buy a server, install Home-AI, configure the house through one interface and keep the essential system working locally without mandatory cloud services, subscriptions or third-party home-automation servers.

Internet connectivity is optional for the core home functions. It may be used for:

- software updates;
- secure remote access;
- uploading personal files from remote clients;
- optional external AI providers;
- explicitly enabled external integrations.

Loss of Internet must not stop local smart-home automation, camera recording, local file access, local AI functions that have a local model, or local administration.

## 2. Final product

Home-AI combines these product domains into one system:

1. **Smart Home**
2. **Cameras / NVR**
3. **File Storage / NAS**
4. **Local AI Agent**
5. **Voice Interface**
6. **Remote Access**
7. **Client File Upload**
8. **Multi-node Cluster**
9. **Unified Web Control Plane**

The user experiences them as one Home-AI installation even when the work is distributed across several physical servers.

## 3. Core principles

### Local-first and autonomous

Home-AI owns the local control path. Cloud services must not be a required dependency for normal operation.

### No mandatory subscriptions

The target product is purchased/installed and operated by its owner. Optional external services may be connected, but the basic product is not designed around recurring third-party subscriptions.

### One logical home, many physical nodes

The first installation may run on one server. The architecture must not assume that one server will always be sufficient.

Additional Home-AI nodes may later contribute:

- CPU;
- RAM;
- GPU/NPU/accelerators;
- storage;
- camera processing;
- AI inference;
- application capacity.

### Modular product

Core remains the trusted control plane. Large features are modules/capabilities rather than permanent hard-coded Core business logic.

Primary future modules include:

- Smart Home;
- NAS / Files;
- Cameras / NVR;
- AI Runtime;
- AI Agent;
- Voice;
- Remote Access / WireGuard;
- Client Sync;
- Cluster.

### Security and audit are mandatory

Every actor — browser, user, device, module, AI agent or cluster node — operates through identity, permissions, policy and audit boundaries.

## 4. Smart Home

Home-AI will implement its **own smart-home runtime**. Home Assistant is not a required platform dependency and is not the target automation engine.

Device support should be modular and may include:

- Zigbee;
- Matter / Thread;
- MQTT;
- Wi-Fi / LAN devices;
- Bluetooth;
- Modbus;
- other local protocols added later.

The Smart Home domain needs:

- a common device/entity/state model;
- local discovery and pairing;
- event handling;
- scenes;
- schedules;
- rules and automations;
- notifications;
- permission-scoped actions;
- room/zone ownership;
- integration with the AI Agent.

The goal is to combine heterogeneous devices into one local model without requiring vendor clouds where local control is technically possible.

## 5. Cameras and NVR

Home-AI includes a full local NVR capability.

Required direction:

- IP/RTSP camera sources;
- ONVIF or equivalent discovery/control where useful;
- live view;
- continuous and event-based recording;
- local archive;
- timeline and event navigation;
- motion/object events;
- AI-assisted video search;
- optional face recognition when supported by available hardware;
- known-person database stored locally;
- retention policies;
- dedicated storage allocation.

### Archive retention

The user allocates a capacity/pool for camera recordings.

Recording continues while capacity is available. When the configured archive space is full, the oldest eligible recordings are overwritten according to retention/protection rules.

Pinned/protected evidence must not be silently overwritten.

## 6. File Storage / NAS

Home-AI includes a real file-storage product rather than only low-level disk administration.

Required direction:

- storage pools/volumes;
- filesystem management;
- Web file manager;
- SMB for Windows clients;
- NFS where needed;
- user quotas where useful;
- snapshots/backup policies where supported;
- health and capacity monitoring.

### User spaces

Every user has:

- private folders visible according to their identity and permissions;
- access to explicitly shared folders;
- optional family/common folders.

Permissions must be enforced by the storage service, not only hidden in the Web UI.

## 7. Client file upload

A Windows client is planned for copying selected user files to Home-AI.

This is **not** a full Windows system-image backup product.

The Windows client should eventually support:

- authenticated pairing with Home-AI;
- selecting folders/files to copy;
- local-LAN and remote operation;
- resumable transfer;
- integrity checking;
- retry after connection loss;
- destination selection inside the user's allowed Home-AI folders;
- scheduled or automatic copying if enabled.

Android is planned later. A dedicated Android application is not an immediate priority.

## 8. AI Agent

### Development sequencing

The **AI Agent foundation starts before the full Smart Home/NVR implementations** so the agent, tool contracts and future domain APIs evolve together.

This does not move Smart Home or NVR business logic into AI. Instead, the agent is developed as a first-party module that consumes explicit typed tools exposed by Core/domain modules. Core remains the trusted control plane for identity, permissions, jobs, events and audit.

The active foundation plan is documented in [AI_AGENT_FOUNDATION.md](AI_AGENT_FOUNDATION.md).

The AI Agent is a central product layer, not merely a chat window.

It should understand authorized context from:

- users;
- rooms and devices;
- automations;
- cameras and recognized events;
- files and metadata;
- server health;
- cluster resources;
- installed modules.

The agent may:

- answer questions about the home;
- control permitted devices;
- search authorized files and camera history;
- diagnose server problems;
- propose new automations;
- learn user preferences;
- improve prompts/tools/policies;
- propose configuration changes;
- coordinate workloads;
- use local LLM/vision models;
- optionally use configured external AI providers.

### Self-development

Long-term, the AI may improve its own Home-AI behavior and propose new tools, modules, automation logic or code changes.

Self-development must be:

- versioned;
- auditable;
- reversible;
- permission-scoped;
- reviewed/approved where required.

The AI must never silently grant itself new permissions.

### Approval model

The AI may make suggestions freely.

Changes to home configuration or automations may be prepared by the AI, but are applied only after approval from the owner/main authorized user unless a previously approved policy explicitly permits that class of action.

Sensitive actions require explicit approval from an authorized user. Examples include:

- deleting files;
- formatting storage;
- changing users/roles;
- changing security policy;
- opening security-critical access;
- disabling alarms;
- changing system/network exposure;
- installing or updating privileged components.

The AI is not an unrestricted root shell.

## 9. Multi-user permissions

Home-AI is a multi-user system.

Permissions may depend on:

- user;
- role;
- room/zone;
- device;
- camera;
- folder/share;
- module/capability;
- action sensitivity.

Examples:

- a child may control devices in their room but not server administration;
- a family member may access shared files but not another user's private folder;
- camera access may differ per user;
- AI tools available to a user are limited by that user's effective permissions;
- owner-only actions remain owner-only even when requested through AI or voice.

## 10. Voice

Voice becomes a major interaction method later.

The intended path is:

```text
microphone terminal
  -> wake word
  -> speech-to-text
  -> Home-AI Agent
  -> policy/permission/approval
  -> Home-AI action
  -> text-to-speech
```

Local STT/TTS is preferred where hardware permits.

Voice terminals are clients of the same identity and permission system rather than privileged shortcuts.

## 11. Remote access

Remote access should use a Home-AI-managed secure channel based on WireGuard or an equivalent explicitly approved secure tunnel.

Goals:

- no direct public exposure of SMB/NFS;
- authenticated device enrollment;
- revocable devices/keys;
- same permission model remotely and locally;
- access to Web, file upload and approved services.

## 12. Multi-node cluster

Multi-node support exists to expand capacity and survive maintenance/failures.

Expected flow:

1. a new Home-AI node is discovered on the local network;
2. the owner explicitly approves enrollment;
3. nodes establish authenticated encrypted trust;
4. the new node advertises CPU/RAM/GPU/storage/capabilities;
5. the cluster scheduler places suitable workloads;
6. the Web UI presents all nodes as one Home-AI system.

The scheduler should eventually distribute:

- AI inference;
- video analytics;
- camera processing;
- movable services;
- background jobs;
- storage-related workloads where locality rules allow it.

When a node fails, Home-AI should reassign workloads where technically possible until the node returns.

Some workloads will remain node-bound because of physical devices or local storage.

### Cluster leadership

The final leader/master model is intentionally not fixed yet.

The architecture must avoid hard-coding a permanent single-server assumption so a later leader-election/failover design remains possible.

## 13. External AI

External AI providers are optional.

They may be used for:

- stronger reasoning;
- development assistance;
- model comparison;
- generation of proposed automations/tools/code;
- tasks that the local hardware cannot reasonably execute.

External AI must not become a required dependency for local home control.

Data sent outside the home must be explicitly controlled and permission-scoped.

## 14. Commercial direction and intellectual property

The project is initially built for the owner's own home, but it should remain suitable for later commercial distribution.

Architecture and dependency decisions must therefore consider:

- ownership of Home-AI source and original components;
- third-party licenses;
- separation from incompatible copyleft/source-available code when necessary;
- reproducible provenance;
- documented modifications;
- signed software/module distribution;
- secure update channels.

The project may study other systems for ideas, but Home-AI must retain its own architecture and implementation boundaries.

## 15. Explicit non-goals

The following are not primary product goals:

- depending on Home Assistant for the smart-home layer;
- depending on vendor clouds for ordinary local control;
- requiring paid external AI for basic operation;
- exposing NAS protocols directly to the public Internet;
- building a generic Proxmox/Portainer replacement as the main product;
- full Windows disk-image backup;
- unrestricted autonomous root access for the AI.

Docker is the standard isolated runtime for independently distributed Home-AI product modules. Core itself remains the trusted control plane and is not intended to become a generic Docker/virtualization management product.

## 16. Product completion criteria

The long-term product vision is achieved when:

- one server can run the complete basic Home-AI system locally;
- smart-home automations keep working without Internet;
- cameras record and can be searched locally;
- user/private/shared files are managed locally;
- a local AI agent can operate the home within permissions;
- voice can control the same functions through the AI/policy layer;
- secure remote access works through the owner's tunnel;
- additional servers can join and contribute resources;
- workloads can be redistributed when a node becomes unavailable where technically possible;
- Web UI presents the entire home as one coherent system;
- external cloud services remain optional rather than mandatory.
