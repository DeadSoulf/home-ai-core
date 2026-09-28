# Home-AI-Core

Home-AI-Core is a modular home-server platform built on top of Debian 13.

The project keeps the base operating system independent and adds a small core responsible for API access, authentication, permissions, events, jobs, updates and module lifecycle management.

## Status

Architecture reset in progress. No production code is accepted until the v1 core architecture and module contract are documented.

## Core principle

The core must not contain Docker, KVM, NAS, AI or NVR-specific logic. Those capabilities belong to independent modules.

See `docs/architecture/` for the current design.
