# Module Registry API v1

Phase 7 exposes read-only module metadata. Installation and update mutations are intentionally deferred to Phase 9.

## List modules

```text
GET /api/v1/modules
```

Requires `modules.read`.

Returns persisted module manifests and registry status.

## Read one module

```text
GET /api/v1/modules/{module_id}
```

Requires `modules.read`.

## Discover capabilities

```text
GET /api/v1/modules/capabilities
```

Requires `modules.read`.

The response contains currently visible platform capabilities.

Initial host capabilities may include:

- `host.linux`
- `host.arch.amd64`
- `host.arch.arm64`
- `host.systemd`
- `host.kvm`
- `host.gpu`

Registered modules may add capabilities declared in their manifests.

Disabled/error modules do not contribute provided capabilities.

Capability names are discovery metadata, not permission grants.

## Registry status

Phase 7 understands these descriptive statuses:

- `registered`
- `enabled`
- `disabled`
- `error`

A registry entry does not prove that a remote package has been downloaded, verified or installed. Package provenance and lifecycle mutations are Phase 9 responsibilities.
