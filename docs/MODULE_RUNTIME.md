# Home-AI external module runtime

**Status:** canonical architecture for new product modules  
**Runtime:** Docker on Debian 13 (`amd64`, `arm64`)

## Boundary

Home-AI-Core is the trusted control plane. Product business logic does not run inside the Core process.

Core owns:

- identity, sessions, users, RBAC and scoped permissions;
- audit, jobs, events and realtime transport;
- node identity and hardware inventory;
- Core updates and privileged typed host operations;
- low-level system/storage foundations that must remain trusted;
- module catalog, manifest validation, compatibility planning and lifecycle orchestration.

Product modules run as independently released Docker containers. Initial module families are:

- Cameras / NVR;
- AI Agent and AI providers;
- Smart Home;
- NAS / Files product services;
- Voice;
- Remote Access;
- Cluster.

## Distribution

Each module has its own source repository, Docker image, version and release lifecycle. A Core update must not be required merely to install or update a compatible module.

A module release publishes a manifest containing at least:

- stable module ID and version;
- compatible Core version range;
- required/provided capabilities;
- permissions/events/UI contribution;
- Docker image reference;
- internal service port;
- health endpoint.

The Core registry can persist an external manifest without loading Go implementation code into the Core process.

## Docker security model

The network-facing `home-ai-core` service MUST NOT receive `/var/run/docker.sock` and MUST NOT be added to the `docker` group. Either grants effective root control of the host.

Container lifecycle operations are delegated to the privileged Home-AI helper through a small typed protocol. The helper accepts only validated module lifecycle operations rather than arbitrary shell or Docker arguments.

Modules do not receive the Docker socket.

Default module containers:

- run with no privileged mode;
- receive no host PID namespace;
- receive no host Docker socket;
- use explicit persistent volumes/directories;
- expose only the internal ports required by the module contract;
- use a dedicated Home-AI module network unless a module has an explicitly reviewed networking requirement;
- receive host devices/mounts only when declared by a future typed capability contract and approved by policy.

## Lifecycle

Target installation flow:

1. Core downloads or receives a signed catalog manifest.
2. Core validates manifest schema, Core compatibility, architecture, dependencies and conflicts.
3. Core requests the privileged helper to pull the exact module image.
4. Helper creates the module-owned data directory/volume and container with the validated policy.
5. Container starts.
6. Core waits for the declared health endpoint.
7. Only a healthy container becomes `enabled` in Module Registry.
8. Failure rolls back the new container while preserving the previous healthy version where possible.

Upgrade follows the same health-gated replacement model. Remove deletes runtime resources only according to the module data-retention policy.

## API and UI integration

A module must not inject arbitrary executable code into the Core process.

Module API and UI integration will use namespaced contracts. The module exposes its service inside the managed module network; Core authenticates users, enforces policy and proxies or bridges only approved namespaced requests.

Navigation contributions remain under `/modules/<module-id>`.

## Persistent data

Core state and module state are separate.

Existing historical AI/NVR tables in the Core database are preserved during the architecture transition so data can be migrated deliberately. New external modules should own their operational data outside the Core SQLite database unless a small shared control-plane record is explicitly part of the Core contract.

## First migration sequence

1. Remove built-in product-module startup/registration from Core.
2. Install Docker with the base Home-AI-Core package.
3. Remove legacy module registry rows without deleting historical data.
4. Add Docker runtime metadata to Module SDK.
5. Implement typed Docker lifecycle operations in the privileged helper.
6. Convert Cameras/NVR into the first external module.
7. Repeat the pattern for AI Agent, Smart Home and the remaining product modules.
