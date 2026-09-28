# ADR-0005: Core API v1 and Realtime Event Protocol

- Status: Accepted
- Date: 2026-09-28

## Context

Home-AI-Core needs a stable API contract for the Web UI, future mobile clients, modules and later multi-node communication.

The REST API must provide authoritative state and commands. Realtime transport is used for transient change notifications and progress signals.

The first realtime implementation must not pretend to provide durable event delivery before the persistent Job/Event Engine exists.

## Decision

### REST API

Stable public APIs use the versioned prefix:

```text
/api/v1/
```

JSON is the default representation.

Every request receives a server-generated `X-Request-ID`.

Clients may send an `X-Correlation-ID` containing a short safe identifier. When valid, Core echoes it. When absent or invalid, Core uses the request ID as the correlation ID.

### Error envelope

API errors use:

```json
{
  "error": {
    "code": "state_unavailable",
    "message": "core state is unavailable",
    "request_id": "...",
    "correlation_id": "..."
  }
}
```

Optional structured `details` may be added when safe.

Internal stack traces, SQL errors and secrets are never returned to clients.

### Realtime endpoint

Realtime events use:

```text
GET /api/v1/events
Upgrade: websocket
```

The implementation uses `github.com/coder/websocket`.

The WebSocket endpoint follows the same origin checks as the library default. Core v1 remains loopback-only until authentication is implemented.

### Event envelope v1

Every server event uses a typed envelope:

```json
{
  "version": 1,
  "id": "evt_...",
  "stream_id": "...",
  "sequence": 42,
  "type": "system.updated",
  "time": "2026-09-28T08:00:00Z",
  "source": {
    "node_id": "...",
    "component": "core"
  },
  "request_id": "...",
  "data": {}
}
```

Fields:

- `version`: event-envelope schema version
- `id`: unique event identifier
- `stream_id`: identifies the current Core process event stream
- `sequence`: monotonically increasing sequence within one stream
- `type`: namespaced event type
- `time`: UTC event creation time
- `source`: origin node/component
- `request_id`: optional triggering request
- `data`: type-specific payload

### Reconnect semantics

Realtime v1 is not a durable event log.

When a connection is lost:

1. client reconnects,
2. client fetches authoritative current state through REST,
3. client re-subscribes to desired realtime topics.

A changed `stream_id` means the Core event stream restarted. Sequence numbers are meaningful only inside one stream.

Replay/resume tokens are deliberately deferred to the persistent Event/Job Engine phase.

### Subscriptions

Client control frames use:

```json
{
  "op": "subscribe",
  "topics": ["system.*", "job.*"]
}
```

Supported operations:

- `subscribe` — add topics
- `unsubscribe` — remove topics

Topic forms:

- exact event type, for example `system.updated`
- namespace wildcard, for example `system.*`
- global wildcard `*`

Control events in the `core.*` namespace are always delivered so the protocol can communicate connection, heartbeat, subscription and protocol-error state.

Subscriptions are transient and are not persisted.

### Heartbeat

Core emits a `core.heartbeat` event every 30 seconds.

Heartbeat is application-level liveness metadata and includes the current UTC timestamp.

The WebSocket protocol implementation may additionally use WebSocket close/ping semantics internally.

### Limits

- inbound WebSocket control message limit: 64 KiB
- maximum topics per connection: 64
- topic maximum length: 128 characters
- invalid control messages receive a `core.error` event
- gross protocol violations may close the WebSocket

### Event naming

Core-reserved namespaces include:

- `core.*`
- `system.*`
- `security.*`
- `module.*`
- `job.*`
- `update.*`

Future modules own their documented namespace, such as `containers.*` or `virtualization.*`.

## Consequences

### Positive

- REST remains the source of truth.
- Web/mobile clients can use one stable realtime envelope.
- Reconnect behaviour is explicit and does not risk silent state divergence.
- Stream and sequence metadata can support diagnostics and later replay evolution.
- Modules and future nodes can publish into the same namespace model.

### Negative

- v1 clients must resync state after reconnect.
- Event persistence and replay remain future work.
- WebSocket adds one direct third-party dependency to the Core.
