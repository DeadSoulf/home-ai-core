# Module SDK v1

Module SDK v1 is the contract between the Home-AI-Core control plane and optional host-level modules.

## Manifest

The canonical machine-readable schema is:

```text
schemas/module-manifest-v1.schema.json
```

Core also performs runtime validation for rules that JSON Schema cannot conveniently express, including:

- Core semantic-version constraint evaluation
- dependency constraints
- dependency cycles
- conflicts in both directions
- module event publish namespace ownership
- module UI route ownership
- architecture compatibility
- capability availability

Unknown JSON fields are rejected.

## Minimal example

```json
{
  "schema_version": 1,
  "id": "storage",
  "name": "Storage",
  "description": "Host storage management capability",
  "version": "0.1.0",
  "core": ">=0.1.0 <1.0.0",
  "permissions": ["system.read"],
  "capabilities": {
    "requires": ["host.linux"],
    "provides": ["storage.block"]
  },
  "host": {
    "architectures": ["amd64", "arm64"]
  },
  "api": {
    "namespace": "storage"
  },
  "events": {
    "publishes": ["storage.changed"],
    "subscribes": ["system.*"]
  },
  "ui": {
    "navigation": [
      {
        "id": "overview",
        "title": "Storage",
        "route": "/modules/storage"
      }
    ]
  },
  "lifecycle": ["install", "upgrade", "remove", "backup", "restore"]
}
```

## Dependency planning

The SDK planner receives:

- target module
- available manifests
- installed module versions
- current Core version
- current host/platform capabilities
- target architecture

The planner produces dependency-first order without mutating the host.

Planning fails before lifecycle execution when:

- a dependency is missing
- a version constraint is not satisfied
- the Core version is incompatible
- a dependency cycle exists
- either side declares a conflict
- a required capability is missing
- the architecture is unsupported

## Lifecycle

The Go SDK exposes a typed `Lifecycle` interface:

- `Install`
- `Upgrade`
- `Remove`
- `Backup`
- `Restore`

Lifecycle methods receive a typed operation context and progress reporter.

Module manifests never contain arbitrary shell commands.

Phase 9 will connect lifecycle orchestration to verified packages, Job Engine execution, rollback and privileged host operations.

## Capabilities

Core discovers a small stable baseline:

- `host.linux`
- `host.arch.amd64` or `host.arch.arm64`
- `host.systemd` when visible
- `host.docker` when the Docker Engine socket is active
- `host.kvm` when `/dev/kvm` is visible
- `host.gpu` when DRM cards are visible

Registered modules can provide additional capabilities.

Capabilities are descriptive planning inputs. They are not authorization permissions.

## Permissions

`permissions` declares Core permissions a module needs.

A declaration does not grant those permissions. Future module identities and grants must be explicit and auditable.

## UI registration

UI contributions are declarative navigation metadata.

Routes must stay below:

```text
/modules/<module-id>
```

Manifest-provided executable JavaScript is not supported.

## Reference module

`internal/modules/demo` implements the complete SDK interface and exists as a build/test reference. It is not automatically registered in production Core.

## Registry API

Read-only registry discovery is documented in `docs/api/modules-v1.md`.

Package discovery, installation, signatures, updates and rollback are intentionally deferred to Phase 9.
