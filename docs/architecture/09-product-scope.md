# Home-AI Product Scope

## Mission

Home-AI is an autonomous local home platform built around one trusted control plane.

It combines:

- Smart Home;
- Cameras / NVR;
- personal File Storage / NAS;
- local AI Agent;
- Voice;
- secure remote access;
- client file upload;
- future multi-node resource sharing.

The system is local-first and does not require mandatory cloud subscriptions for its essential home functions.

## Smart Home

Home-AI implements its own:

- device/entity/state model;
- rooms/zones;
- automations;
- scenes/schedules;
- permission-aware actions.

Protocol support is modular: Zigbee, Matter/Thread, MQTT, Wi-Fi/LAN, Bluetooth, Modbus and future local integrations.

Home Assistant is not a required dependency.

## Cameras / NVR

- IP/RTSP cameras;
- live view;
- local recording;
- configurable archive allocation;
- retention and oldest-first overwrite;
- event timeline;
- motion/object detection;
- optional hardware-backed face recognition;
- AI-assisted search/analysis.

## File Storage

- private per-user folders;
- shared folders;
- Web file manager;
- SMB/NFS where appropriate;
- storage pools/volumes;
- health/capacity;
- snapshots/backup policies where supported.

## AI Agent

The AI Agent is a first-class system actor.

It uses approved tools and the effective permissions of the requesting user.

AI may learn preferences, propose automations/configuration and coordinate Home-AI capabilities.

Changes requiring approval remain pending until approved by the appropriate authorized user.

AI cannot silently escalate its own permissions or bypass audit/security boundaries.

External AI is optional.

## Multi-node

Additional Home-AI servers may contribute CPU, RAM, GPU, storage and workload capacity.

The product should eventually distribute movable workloads and keep operating in a degraded mode when another node is unavailable.

## Voice and clients

Voice becomes a primary interaction mode later.

A Windows client is planned first for copying selected user files to Home-AI. Android follows later.

Remote access should use an owner-controlled secure tunnel such as WireGuard.

## Core non-goals

Core itself is not the place for all product business logic.

Core provides the shared contracts:

- identity/security/policy;
- state;
- jobs;
- events;
- audit;
- modules;
- updates;
- node/resource inventory.

NAS, Smart Home, NVR, AI, Voice and Cluster functionality should remain modular.

## Scope explicitly deprioritized

- Home Assistant as the smart-home engine;
- generic virtualization as a primary product;
- generic Docker-management UI as a primary product;
- generic third-party app marketplace as a primary product;
- full Windows system-image backup;
- mandatory external AI/cloud dependency.
