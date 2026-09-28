# Jobs & Durable Events Acceptance Criteria

Phase 6 is complete when long-running work and domain events are durable, inspectable and connected to the existing security/realtime contracts.

## Persistent jobs

- [x] SQLite jobs schema exists
- [x] job IDs are stable
- [x] jobs are node-aware
- [x] actor/request/correlation metadata is stored
- [x] queued/running/cancel-requested/cancelled/succeeded/failed states exist
- [x] progress uses bounded basis points
- [x] results/errors are persisted
- [x] interrupted running jobs are recoverable after Core restart

## Execution engine

- [x] typed handler registry exists
- [x] arbitrary command strings are not accepted as handlers
- [x] bounded in-process workers exist
- [x] queued work is claimed from persistent state
- [x] progress reporting is supported
- [x] cooperative context cancellation is supported
- [x] missing handlers fail jobs safely
- [x] worker state does not bypass Core permissions

## Durable events

- [x] events are persisted before live publication
- [x] each event has a stable ID
- [x] each event has a monotonic local SQLite cursor
- [x] node/component/actor/request/job metadata can be recorded
- [x] durable cursor is included in realtime envelopes
- [x] protocol-only realtime events remain non-durable

## API

- [x] `GET /api/v1/jobs`
- [x] `GET /api/v1/jobs/{id}`
- [x] `POST /api/v1/jobs/{id}/cancel`
- [x] `GET /api/v1/events/history?after=<cursor>`
- [x] jobs APIs are protected by RBAC
- [x] cancellation requires `jobs.cancel`
- [x] cookie-based cancellation uses CSRF protection
- [x] event history requires `events.read`

## Validation

- [x] migration version updated to 3
- [x] migration idempotency tests pass
- [x] typed job engine integration test passes
- [x] queued cancellation test passes
- [x] durable job events are verified
- [x] `go test ./...`
- [x] `go vet ./...`
- [x] daemon smoke test
- [x] linux/amd64 build
- [x] linux/arm64 build

## Deferred

The following remain future work:

- cross-node job scheduling
- exactly-once distributed execution
- arbitrary module registration before the Module SDK
- long-term event retention/compaction policy
- high-frequency telemetry/event storage
- camera/media payload transport
