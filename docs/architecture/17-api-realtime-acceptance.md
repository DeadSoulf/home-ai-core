# Core API v1 / Realtime Acceptance Criteria

Phase 4 is considered complete when the first stable Core transport contracts are implemented and validated.

## REST contract

- [x] stable `/api/v1/` namespace
- [x] server-generated request IDs
- [x] client correlation IDs with validation/fallback
- [x] JSON error envelope
- [x] not-found errors use the error envelope
- [x] unsupported API methods use the error envelope
- [x] internal errors are not exposed to clients

## Realtime transport

- [x] `GET /api/v1/events` WebSocket upgrade
- [x] Event Envelope v1
- [x] unique event IDs
- [x] per-connection stream IDs
- [x] strictly increasing per-connection sequence numbers
- [x] event source includes node identity
- [x] triggering request ID can be carried into events
- [x] `subscribe` control operation
- [x] `unsubscribe` control operation
- [x] exact, namespace wildcard and global wildcard topics
- [x] `core.*` protocol events are always delivered
- [x] application heartbeat
- [x] bounded client event queue
- [x] slow consumers are disconnected
- [x] inbound control-message size limit
- [x] malformed/unsupported control operations return `core.error`

## Reconnect model

- [x] REST remains authoritative state
- [x] replay is explicitly not promised in realtime v1
- [x] reconnect requires REST resync and re-subscription
- [x] each new WebSocket connection receives a new stream ID

## Validation

- [x] topic matching tests
- [x] sequence ordering tests
- [x] subscribe/publish WebSocket test
- [x] invalid-control-message test
- [x] API WebSocket route integration test
- [x] request/correlation tests
- [x] API error-envelope tests
- [x] `go vet`
- [x] daemon smoke test
- [x] linux/amd64 build
- [x] linux/arm64 build

## Deferred

Durable events, replay/resume tokens, persistent jobs, event retention and cross-node event distribution remain part of the later Job/Event Engine and multi-node phases.
