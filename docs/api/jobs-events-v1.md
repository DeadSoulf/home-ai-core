# Jobs and Durable Events API v1

## Jobs

### List jobs

```text
GET /api/v1/jobs?status=<optional>&limit=100
```

Requires `jobs.read`.

Returns persistent jobs ordered newest first.

### Read one job

```text
GET /api/v1/jobs/{job_id}
```

Requires `jobs.read`.

### Request cancellation

```text
POST /api/v1/jobs/{job_id}/cancel
```

Requires `jobs.cancel`.

When cookie authentication is used, a valid `X-CSRF-Token` is also required.

Cancellation is cooperative:

- queued jobs are cancelled before start
- running jobs transition through `cancel_requested`
- handlers receive context cancellation and must stop at a safe boundary

## Durable event history

```text
GET /api/v1/events/history?after=<cursor>&limit=100
```

Requires `events.read`.

Returns durable domain events in ascending cursor order.

The response also contains `latest_cursor`.

The durable cursor is local to the node database. It is not a future cluster-wide consensus sequence.

## WebSocket relationship

Durable events are persisted before live publication.

When the same durable event is delivered over `/api/v1/events`, its envelope contains the durable `cursor`.

Protocol-only realtime messages such as `core.connected` and `core.heartbeat` have no durable cursor.

## Reconnect

Recommended flow:

1. remember the last durable cursor processed
2. reconnect and refresh authoritative REST state
3. fetch `/api/v1/events/history?after=<last_cursor>`
4. reconnect/re-subscribe to WebSocket topics

This is catch-up, not exactly-once distributed delivery.
