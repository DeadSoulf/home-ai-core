# Core API v1

Core API v1 is the first stable contract surface for the restarted Home-AI-Core platform.

## Transport rules

- REST base path: `/api/v1/`
- JSON responses use UTF-8
- API responses are marked `Cache-Control: no-store`
- every request receives `X-Request-ID`
- clients may provide a safe `X-Correlation-ID`, which Core echoes
- default listener remains loopback-only until authentication is implemented

## GET /health

Purpose:

- local process health checks
- systemd/reverse-proxy probes
- installation verification
- Core state database readiness

Response:

```json
{
  "status": "ok",
  "version": "0.1.0-dev"
}
```

If the Core state database is unavailable, the endpoint returns `503 Service Unavailable` with `status: degraded`.

The health endpoint is operational infrastructure and does not use the normal API error envelope.

## GET /api/v1/system

Returns read-only information about the local node.

Initial information includes:

- stable node ID
- hostname
- operating system and kernel
- CPU model and logical CPU count
- total and currently available memory
- uptime
- whole block devices visible in `/sys/block`
- network interfaces and addresses
- basic DRM/PCI GPU discovery
- Core version
- Core database schema version

Block-device data deliberately does not create permanent identities from `/dev/*` paths.

## API errors

Example:

```json
{
  "error": {
    "code": "not_found",
    "message": "resource not found",
    "request_id": "95df...",
    "correlation_id": "mobile-upload-42"
  }
}
```

Error codes are stable machine-readable identifiers. Messages are safe for users/logging. Internal errors are not returned.

## GET /api/v1/events

Upgrades to WebSocket and provides Event Envelope v1.

The server first emits `core.connected`.

A client may then subscribe:

```json
{
  "op": "subscribe",
  "topics": ["system.*", "job.*"]
}
```

or unsubscribe:

```json
{
  "op": "unsubscribe",
  "topics": ["system.*"]
}
```

Core protocol events are always delivered.

### Event envelope

```json
{
  "version": 1,
  "id": "evt_...",
  "stream_id": "stream_...",
  "sequence": 10,
  "type": "core.heartbeat",
  "time": "2026-09-28T08:00:00Z",
  "source": {
    "node_id": "...",
    "component": "core"
  },
  "request_id": "...",
  "data": {}
}
```

### Heartbeat

`core.heartbeat` is emitted every 30 seconds.

### Reconnect

Realtime v1 does not persist or replay events.

After reconnect, clients must:

1. fetch current authoritative state using REST,
2. reconnect to `/api/v1/events`,
3. restore their subscriptions.

A changed `stream_id` indicates that the Core event stream restarted.

Persistent event history/replay belongs to the later Event/Job Engine.
