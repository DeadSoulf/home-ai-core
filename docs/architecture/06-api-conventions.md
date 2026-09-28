# API Conventions

## Base path

All stable API endpoints are versioned:

```text
/api/v1/
```

Breaking changes require a new major API version.

## Resource style

Examples:

```text
GET    /api/v1/system
GET    /api/v1/modules
POST   /api/v1/modules/{id}/install
POST   /api/v1/modules/{id}/remove
GET    /api/v1/jobs/{id}
GET    /api/v1/audit
```

Module APIs are namespaced:

```text
/api/v1/modules/{module-id}/...
```

## Long-running operations

Operations that may outlive a normal request return a Job reference rather than holding the HTTP connection open.

Example response:

```json
{
  "job_id": "01H...",
  "status": "queued"
}
```

The normal HTTP status for accepted asynchronous work is `202 Accepted`.

## Events

Realtime status is delivered through a versioned WebSocket/event channel.

Initial event families include:

- `system.*`
- `module.*`
- `job.*`
- `security.*`
- `update.*`

Modules own their namespace, for example `containers.*` or `virtualization.*`.

## Errors

Errors use a stable machine-readable shape with:

- code
- message
- request_id
- optional details

Internal stack traces are never returned to normal clients.

## Requests

Every request receives a correlation/request ID.

Mutating APIs should support idempotency where repeated submission could otherwise create duplicate resources.
