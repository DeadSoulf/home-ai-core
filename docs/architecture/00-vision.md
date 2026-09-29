# Home-AI-Core — Architecture Vision

Home-AI-Core is the trusted control-plane foundation of the Home-AI product.

The canonical product requirements are defined in `../PRODUCT_VISION.md`.

## Product model

```text
Debian
  |
Home-AI-Core
  |
  +-- Identity / Security / Policy / Audit
  +-- State / Jobs / Events / Realtime
  +-- Module Registry / Update Manager
  +-- Hardware / Node Capability Inventory
  +-- Web UI
  |
  +-- Home-AI Modules
       +-- File Storage / NAS
       +-- Smart Home
       +-- Cameras / NVR
       +-- AI Runtime / AI Agent
       +-- Voice
       +-- Remote Access / WireGuard
       +-- Client Sync
       +-- Cluster
```

## Architectural goal

A single Home-AI installation starts on one Debian server but can later grow into several trusted nodes while remaining one logical home system.

The product must remain locally operable without mandatory cloud services.

## Core boundary

Core contains shared trusted contracts and orchestration.

Core should not permanently absorb product-specific NVR, NAS, Smart Home or model-inference business logic.

Those domains use Core identity, permissions, jobs, events, audit and module/update contracts.

## AI boundary

The AI Agent is powerful but is not a security bypass.

AI actions execute through the same permission and approval system used by other clients and are auditable.

## Cluster boundary

Single-node operation is the first implementation target.

APIs and resource identity should avoid assumptions that prevent later node enrollment, resource advertisement, workload scheduling and failover.
