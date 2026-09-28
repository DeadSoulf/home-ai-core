# ADR-0007: Persistent Job Engine and Durable Event Bus

- Status: Accepted
- Date: 2026-09-28

## Context

Home-AI-Core must perform operations that outlive an HTTP request: backups, package installation, storage operations, container pulls, VM operations and AI/model tasks.

The platform also needs events that survive process restarts and can be inspected after a WebSocket disconnect.

Phase 4 intentionally made the WebSocket stream non-durable. Phase 6 adds persistence without changing that transport guarantee.

## Decision

### Jobs

Long-running work is represented by persistent jobs.

Job states are:

```text
queued
running
cancel_requested
cancelled
succeeded
failed
```

A job records:

- stable job ID
- job type
- node ID
- actor type/ID
- request/correlation ID
- status
- progress from 0 to 10000 basis points
- safe status message
- created/started/completed timestamps
- cancellation timestamp
- structured input/result JSON where the owning job type explicitly allows it
- safe error code/message

Secrets must not be placed in generic job payloads.

### Execution

The Core Job Engine owns a registry of typed handlers.

Modules will later register handlers through the Module SDK rather than submitting arbitrary command strings.

Queued work is claimed from persistent state. A job that was left `running` by a process crash is returned to `queued` during engine recovery unless its future handler policy states otherwise.

The first engine uses bounded in-process workers. Distribution across nodes is deferred to the multi-node scheduler phase.

### Cancellation

Cancellation is cooperative.

- queued jobs may become `cancelled` before execution
- running jobs become `cancel_requested`
- the engine cancels the handler context
- the handler must stop at a safe boundary
- cancellation never means force-killing an arbitrary host process

### Durable events

Domain events are persisted before live publication.

Each durable event has:

- SQLite monotonic cursor
- stable event ID
- node ID
- component
- type
- actor identity where available
- request/correlation ID
- optional job ID
- UTC timestamp
- JSON payload

The cursor is a local durable ordering primitive, not a future cluster-wide consensus sequence.

### Live delivery

The existing WebSocket Event Envelope v1 remains the live transport.

Persistent event ID/time/source are forwarded into the WebSocket message. Each WebSocket connection still has its own `stream_id` and delivery `sequence`.

Protocol-only events such as `core.connected` and `core.heartbeat` are not written to the durable event log.

### History

Authenticated clients may read durable event history through:

```text
GET /api/v1/events/history?after=<cursor>&limit=<n>
```

This supports diagnostic/reconnect catch-up without claiming exactly-once delivery.

Recommended client reconnect flow becomes:

1. remember last durable cursor,
2. reconnect/fetch authoritative REST state,
3. fetch event history after that cursor if needed,
4. re-establish WebSocket subscriptions.

REST state remains authoritative.

### Permissions

Phase 6 introduces:

- `jobs.read`
- `jobs.cancel`

The existing `events.read` permission covers durable event history.

## Consequences

### Positive

- Long operations survive request lifetimes.
- Job progress and failure state are inspectable after reconnect/restart.
- Domain events become auditable/diagnosable without turning WebSocket into a database.
- Current single-node implementation is compatible with a later node-aware scheduler.

### Negative

- The Core database receives more write traffic.
- Event retention/compaction policy is required before high-volume camera/telemetry events use this bus.
- Distributed ordering and distributed execution remain separate future problems.

## Explicit non-goals

Phase 6 does not provide:

- arbitrary command execution jobs
- cross-node scheduling
- exactly-once distributed delivery
- infinite event retention
- high-frequency telemetry storage
- camera-frame or media payload transport
