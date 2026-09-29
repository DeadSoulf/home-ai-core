# Core API v1

Core API v1 is the authenticated control-plane surface for the local Home-AI-Core node.

## Transport rules

- REST base path: `/api/v1/`
- JSON uses UTF-8
- authenticated browser requests use same-origin cookie sessions
- state-changing cookie requests require CSRF validation
- every request receives `X-Request-ID`
- safe client `X-Correlation-ID` values are echoed
- API errors use stable machine-readable codes

## Public operational endpoint

### GET /health

Returns Core/database readiness and the running Core version.

The health endpoint is intentionally outside the normal authenticated API envelope so systemd/reverse-proxy/local recovery tooling can probe it.

## Authentication and setup

Current security routes include:

- `GET /api/v1/security/setup-status`
- `POST /api/v1/security/bootstrap`
- `POST /api/v1/auth/login`
- `GET /api/v1/auth/me`
- `POST /api/v1/auth/logout`

The first-owner bootstrap remains local-only.

## System inventory

### GET /api/v1/system

Requires `system.read`.

Returns the local node identity and current inventory including:

- hostname, OS, kernel and architecture
- CPU utilization/model/count
- memory
- uptime
- recursive block-device tree
- filesystem/unallocated-space information
- persistent disk display names
- SMART/LVM metadata when the privileged helper can provide it
- network interfaces
- GPU/PCI inventory
- Core/schema versions

The unprivileged Core collects safe inventory and merges privileged storage inspection returned by the helper.

## Storage

Storage mutations require `storage.manage` and CSRF for cookie sessions.

- `POST /api/v1/storage/operation`
- `POST /api/v1/storage/name`

Supported low-level operations currently include mount/unmount, format, partition create/delete/delete-all and filesystem-label changes. The privileged helper enforces system-disk protection and validates each operation again.

## Updates

Reading updater information requires `updates.read`.

- `GET /api/v1/update`
- `GET /api/v1/update/state`

Mutations require `updates.manage`:

- `POST /api/v1/update/download`
- `POST /api/v1/update/install`
- `POST /api/v1/update/rollback`

A manual fresh release check uses `GET /api/v1/update?fresh=1`; normal callers can use the cached status.

## Jobs, events, modules and audit

The current API also exposes:

- persistent Jobs
- durable Event history
- realtime WebSocket events
- module registry/capabilities
- audit events

See the dedicated API documents for their detailed contracts.

## Error envelope

Example:

```json
{
  "error": {
    "code": "not_found",
    "message": "resource not found",
    "request_id": "95df...",
    "correlation_id": "client-operation-42"
  }
}
```

Error codes are machine-readable. Internal implementation details are logged server-side rather than exposed indiscriminately.
