# Cluster Core

## Goal

Cluster Core lets multiple Home AI Core servers operate as one logical pool.

The initial implementation uses a controller/worker model:

- **Controller** receives authenticated heartbeats from workers, keeps the node inventory and
  makes placement decisions.
- **Worker** reports local CPU, RAM and root-filesystem utilization to the controller.
- A standalone installation keeps cluster mode disabled and behaves exactly as before.

## Configuration

The same shared token must be configured on every node. The token authenticates heartbeat
messages but does not encrypt traffic. Run cluster traffic only on a trusted LAN or over
WireGuard.

Controller example:

```text
cluster.enabled=true
cluster.role=controller
cluster.node_id=server-01
cluster.node_name=Primary server
cluster.advertise_address=10.10.0.10
cluster.shared_token=<same-secret-on-all-nodes>
```

Worker example:

```text
cluster.enabled=true
cluster.role=worker
cluster.node_id=server-02
cluster.node_name=Worker 02
cluster.advertise_address=10.10.0.11
cluster.controller_host=10.10.0.10
cluster.controller_port=8080
cluster.shared_token=<same-secret-on-all-nodes>
```

A restart is required after changing cluster settings.

## Heartbeat

Workers POST to:

```text
POST /api/cluster/heartbeat
```

with the `X-HomeAI-Cluster-Token` header. The heartbeat includes node identity plus current
CPU, RAM and system-disk utilization.

The controller marks a remote node offline after `cluster.timeout_seconds` without a heartbeat.

## Placement scheduler

The controller exposes:

```text
GET /api/cluster/placement?workload=generic
GET /api/cluster/placement?workload=ai
GET /api/cluster/placement?workload=cameras
GET /api/cluster/placement?workload=vm
```

Placement uses weighted CPU, RAM and disk load. Nodes above critical thresholds are excluded.
Different workload types use different weights.

This version provides the common scheduler and node-health layer. It does **not** silently move
already-running cameras, VMs, files or AI jobs between servers. Those modules must explicitly
use the placement API when creating or assigning new work. Live migration and distributed
storage require separate coordination and safety rules.

## Security

Cluster inventory requires `cluster.view`. Cluster configuration requires `cluster.manage`.

Heartbeat authentication is independent from browser sessions. Keep the shared token secret and
route cluster traffic over a private network or WireGuard.
