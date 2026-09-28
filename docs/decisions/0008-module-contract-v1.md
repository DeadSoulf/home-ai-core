# ADR-0008: Module Contract v1

- Status: Accepted
- Date: 2026-09-28

## Context

Home-AI-Core must grow through optional host-level modules without turning Core into a monolith.

Containers, Storage, NAS, Virtualization, AI and NVR must be independently versioned and removable while still sharing the Core security, jobs, events and UI shell.

Phase 7 defines the module contract. It does not yet define a downloadable package repository, signatures or host package installation; those belong to Phase 9.

## Decision

### Manifest format

Module metadata is declared as JSON using schema version 1.

A manifest declares:

- stable module ID
- human-readable name and description
- semantic module version
- compatible Core version constraint
- module dependencies and conflicts
- Core permissions required by the module
- host capabilities required by the module
- capabilities provided by the module
- OS architectures and package requirements
- API namespace
- events published/subscribed
- UI navigation contributions
- supported lifecycle operations

The manifest is data, not executable installation code.

### Identity

Module IDs use lowercase dotted or dashed identifiers such as:

- `containers`
- `storage`
- `ai.runtime`
- `network-vpn`

An ID is stable across versions.

### Versions and constraints

Module versions use semantic `major.minor.patch` form.

Version constraints support whitespace-separated comparisons:

```text
>=0.1.0 <1.0.0
```

Dependencies use the same constraint syntax.

### Dependencies and conflicts

Dependencies form a directed acyclic installation graph.

Before any future install operation, Core must reject:

- missing dependencies
- version-incompatible dependencies
- circular dependencies
- declared conflicts with installed/planned modules
- incompatible Core versions

The planner returns dependency-first ordering.

### Permissions

A module declares the Core permissions it needs.

Declaration is not equivalent to a permission grant.

Future module identities/grants must be explicit and auditable. Modules never inherit owner permissions.

### Capabilities

Capabilities are stable names such as:

```text
host.linux
host.systemd
storage.block
runtime.containers
accelerator.gpu
```

A manifest may require host/platform capabilities and provide new capabilities.

The planner refuses activation when required capabilities are unavailable.

### Lifecycle contract

SDK v1 defines these operations:

- install
- upgrade
- remove
- backup
- restore

A module declares which operations it supports.

The SDK contract is typed; arbitrary shell command strings are not part of the manifest.

Actual lifecycle execution, rollback, package verification and privileged host mutation are deferred to Module Store/Updates and the privileged-helper phases.

### API namespace

A module may reserve:

```text
/api/v1/modules/<module-id>/
```

The manifest may declare a shorter logical namespace, but a module may not claim Core-owned paths.

### Events

Module event types must use the module's namespace or one of its explicitly owned capability namespaces.

Core protocol namespaces remain reserved.

### UI contributions

Modules register declarative navigation entries.

A contribution contains:

- stable entry ID
- title
- route
- optional icon key
- order

The Core Web UI decides final rendering and does not execute arbitrary manifest-provided JavaScript.

### Persistent registry

Core persists registered module manifests and status.

Phase 7 statuses are descriptive registry states:

- registered
- enabled
- disabled
- error

A registered module is not evidence that an external package was downloaded or installed.

## Security

- manifests are strictly validated before registration
- unknown JSON fields are rejected
- module permissions are declarations, not implicit grants
- UI contributions are declarative
- no lifecycle shell commands are accepted in manifests
- package signatures and provenance are required before Phase 9 permits remote installation

## Consequences

### Positive

- Core remains independent of Docker/KVM/NAS/AI implementation details.
- Dependency resolution is deterministic before host mutation.
- Web UI can discover installed capabilities without hard-coded sections.
- Module Store can later reuse the same manifest contract.

### Negative

- Manifest v1 is intentionally conservative.
- Runtime module isolation and signed distribution remain future work.
- Complex package-manager behavior is not represented in Phase 7.
