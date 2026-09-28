# Core API v1 — Initial Surface

This document describes the first read-only API surface implemented by Core v0.1.

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

Block-device data currently describes kernel-visible whole devices and deliberately ignores loop/ram/zram devices. Stable storage resource identities will be introduced by the Storage module rather than treating a `/dev/*` path as permanent identity.

GPU discovery is intentionally basic. It reports vendor/device IDs, driver and PCI address where Linux sysfs exposes them. Accelerator/runtime-specific inventory belongs to later AI/Video modules.

This endpoint is intentionally read-only.

Authentication is not implemented in Core v0.1; until Phase 5 identity/security work is complete, the default listener remains loopback-only.

## Request IDs

Core adds an `X-Request-ID` response header to requests.

Future structured errors and audit records will reference the same request identity.
