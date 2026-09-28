# Core API v1 — Initial Surface

This document describes the first read-only API surface implemented by Core v0.1.

## GET /health

Purpose:

- local process health checks
- systemd/reverse-proxy probes
- installation verification

Response:

```json
{
  "status": "ok",
  "version": "dev"
}
```

No privileged information is returned.

## GET /api/v1/system

Returns basic read-only node information.

Initial fields include:

- stable node ID
- hostname
- operating system description
- architecture
- logical CPU count
- total memory when available
- uptime when available
- Core version

This endpoint is intentionally read-only.

Authentication is not implemented in Core v0.1; before the API is exposed beyond loopback, Phase 5 identity/security work must be completed.

## Request IDs

Core adds an `X-Request-ID` response header to requests.

Future structured errors and audit records will reference the same request identity.
