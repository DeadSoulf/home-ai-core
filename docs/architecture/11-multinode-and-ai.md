# Multi-node and AI Architecture

## Multi-node requirement

Home-AI-Core v1 development begins on a single node, but Core identities and APIs are designed so that multi-node support can be added without replacing the data model.

Every managed resource should have a stable identity that can later include a node owner.

Example conceptual identity:

```text
home
  └── node
       └── resource
```

## Node model

A node advertises capabilities such as:

- CPU architecture and capacity
- memory
- GPUs/accelerators
- block storage
- network interfaces
- container runtime availability
- virtualization availability
- camera-processing capabilities

The future scheduler selects nodes according to declared requirements and available capacity.

## Cluster principles

1. A node failure must not corrupt cluster-wide control state.
2. Workloads explicitly declare whether they are movable or node-bound.
3. Storage locality is explicit.
4. GPU and device passthrough workloads are usually node-bound.
5. Cluster communication is authenticated and encrypted.
6. A household must be operable in degraded mode when another node is offline.
7. Cluster membership and node trust are explicit administrative actions.

## AI Core role

The AI layer is a privileged *client* of platform APIs, not a bypass around them.

AI may:

- inspect system health exposed by approved APIs
- propose configuration changes
- execute pre-authorized low-risk actions
- request approval for sensitive operations
- orchestrate smart-home actions within policy
- use selected user data for search/reasoning when permission is granted

AI may not directly:

- obtain an unrestricted root shell
- read arbitrary private files without scope
- alter permissions or audit records
- silently expose services to the Internet
- bypass human approval requirements

## AI action flow

```text
User / Automation
       |
       v
     AI Core
       |
       v
Tool / Capability Request
       |
       v
Policy + Permission Engine
       |
       +--> allowed automatically
       |
       +--> requires approval
       |
       +--> denied
       |
       v
Core / Module API
       |
       v
Audited action
```

This allows AI to become deeply useful without making it a hidden superuser.
