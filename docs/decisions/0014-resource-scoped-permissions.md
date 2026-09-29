# ADR-0014: Resource-Scoped Permissions

- Status: Accepted
- Date: 2026-09-29

## Context

The initial RBAC model answers only whether an actor has a global permission such as `storage.manage`.

The Home-AI product needs narrower authorization:

- a user may control one room but not another;
- a user may view selected cameras;
- private and shared folders need different membership;
- AI tools must inherit the effective permissions of the requesting user;
- future cluster/node operations may be limited to selected resources.

Encoding resource IDs into permission names would make the permission catalog unstable and difficult to audit.

## Decision

Keep the existing global permission model and add explicit resource-scoped grants.

A scoped grant contains:

```text
permission + resource_type + resource_id
```

Examples:

```text
files.read   + folder + folder_alice
camera.read  + camera + camera_driveway
home.control + room   + room_child
```

Global permissions remain unchanged.

If an actor has a global permission, it authorizes that operation regardless of resource scope.

Otherwise the actor must have an exact scoped grant for the requested resource.

## Storage model

Two grant sources are supported:

- role-scoped grants;
- direct user-scoped grants.

This keeps normal RBAC reusable while allowing exceptions for a specific user without creating artificial per-user roles.

Effective session authorization is the union of:

- global permissions inherited from roles;
- resource grants inherited from roles;
- direct user resource grants.

## Exact matching

The first scoped-permission contract uses exact matching.

There are intentionally no:

- wildcard resource IDs;
- negative/deny grants;
- implicit resource hierarchy;
- automatic inheritance from a room to its devices.

Domain services own hierarchy and membership. For example, the Smart Home domain may determine that a device belongs to a room and then evaluate the relevant grants explicitly.

This avoids teaching the generic security layer product-specific topology.

## Actor contract

`Actor.Has(permission)` remains the compatibility API for global permission checks.

`Actor.Allows(permission, resourceType, resourceID)` returns true when:

1. the actor has the permission globally; or
2. an exact scoped grant matches all three values.

Future API/domain handlers should use `Allows` when operating on scoped resources.

## AI

The AI Agent does not receive broader authorization than its effective actor context.

An AI tool call on behalf of a user must pass the same global/scoped authorization check as a direct API request.

Approval policy is a separate layer and can only further restrict execution; approval does not create authorization.

## Consequences

### Positive

- existing owner/global permissions remain compatible;
- rooms, devices, cameras and folders can later be isolated per user;
- AI authorization can reuse the same model;
- RBAC remains small and understandable;
- domain hierarchy stays outside the security kernel.

### Negative

- APIs must identify the resource before authorization can complete;
- domain services must explicitly evaluate hierarchy when needed;
- management UI/API for scoped grants is still required.

## Deferred

This ADR does not yet implement:

- user/role management UI;
- wildcard scopes;
- deny rules;
- room-to-device inheritance;
- approval workflows;
- non-user actors such as AI/device/node identities.
