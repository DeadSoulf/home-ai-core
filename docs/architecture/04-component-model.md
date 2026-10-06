# Component Model

## Core

The Core is the smallest trusted layer. It owns:

- API routing/version negotiation;
- authentication/sessions;
- authorization;
- audit;
- jobs/events/realtime;
- module registry;
- update orchestration;
- basic host capability discovery;
- storage/network foundations;
- privileged-helper protocol;
- Web shell.

Core contains no product-specific runtime.

## Module

A Module is an independently distributed workload packaged as a Docker image.

A module declares:

- stable ID;
- version;
- Core compatibility;
- image/digest;
- supported architectures;
- capabilities and permissions;
- dependencies/conflicts;
- API namespace;
- UI/navigation contribution;
- events;
- volumes;
- network requirements;
- host/device requirements;
- health check;
- lifecycle operations.

## Runtime boundary

Core stores module metadata and desired/observed lifecycle state. Domain data belongs to the module.

The Core service does not mount `docker.sock`. The privileged helper performs tightly validated lifecycle operations.

## Dependency rules

- Core does not depend on optional modules.
- Modules may depend on explicit Core capabilities or other modules.
- Circular dependencies are invalid.
- Removal is blocked while another installed module has a declared dependency.
