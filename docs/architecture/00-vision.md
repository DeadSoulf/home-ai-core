# Home-AI-Core — Architecture Vision

Home-AI-Core is the trusted control-plane foundation of Home-AI.

The canonical product requirements are defined in `../PRODUCT_VISION.md`.

## Product model

```text
Debian
  |
Home-AI-Core
  +-- Identity / Security / Policy / Audit
  +-- State / Jobs / Events / Realtime
  +-- Module Registry / Core Update Manager
  +-- Hardware / Node Capability Inventory
  +-- Storage / Network foundation
  +-- Web UI shell
  |
  +-- Docker Module Runtime
       +-- external module
       +-- external module
       +-- external module
```

## Core boundary

Core contains shared trusted contracts and orchestration only.

Functional workloads are separate modules. They have independent repositories, images, versions, state and lifecycle. Core must not absorb module-specific business logic or runtime dependencies.

## Privilege boundary

The network-facing Core remains unprivileged. It does not receive the Docker socket. Privileged host/container operations are delegated to the restricted helper through explicit validated operations.

## Growth

Single-node operation is the first target. Stable resource identity and module contracts should allow future multi-node orchestration without replacing the security model.
