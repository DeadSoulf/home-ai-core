# ADR-0031: Unified household identities and per-user access

- Status: Accepted for implementation
- Date: 2026-09-30

## Context

Home-AI already has a single Core user table, sessions, RBAC permissions and exact resource-scoped grants. The first household slice exposed only the broad roles `owner` and `member`, while product modules are becoming more specific: files, storage, network, cameras, rooms, NVR and AI tools need one consistent human identity and a configurable access policy.

Creating separate users inside NAS, NVR, Smart Home or other modules would fragment identity, duplicate passwords and make audit/AI authorization unreliable.

The household also needs understandable starting profiles such as administrator, parent, child, guest and friend without making those labels rigid security boundaries.

## Decision

There is one human account per person in Home-AI-Core. That account is the identity used by Web, files, SMB mapping, future Smart Home, cameras/NVR, AI and other Core modules.

Home-AI exposes five household profiles:

- `administrator`
- `parent`
- `child`
- `guest`
- `friend`

Profiles are templates and labels. Except for Administrator, they are not fixed permission bundles. The administrator can customize each user's global capabilities and exact resource grants.

The stable legacy role IDs are preserved for compatibility:

- `role_owner` becomes the product profile `administrator`;
- `role_member` becomes `friend`;
- new role IDs represent parent, child and guest.

Administrator remains a full-access invariant. The final enabled administrator cannot be disabled or demoted, and an administrator cannot remove their own administrator access.

A new `user_permissions` table stores direct global permissions. Effective authorization becomes the union of:

- profile/role permissions;
- direct user global permissions;
- profile/role resource grants;
- direct user resource grants.

The existing `Actor.Has` and `Actor.Allows` contracts remain unchanged for callers.

Every active non-administrator profile always receives the minimum identity/session permissions:

- `security.self.read`;
- `security.sessions.manage`.

The Web user editor exposes the complete current Core permission catalog. Selecting Parent, Child, Guest or Friend applies a starting template; the administrator can then add or remove individual capabilities.

Resource access remains exact and object-scoped. The first live catalog exposes NAS `file_folder` resources with `files.read` and `files.write`. Future room, camera, NVR and AI resources use the same tuple:

`permission + resource_type + resource_id`

Shared NAS folders no longer inherit access from the old global `member` role. Existing member shared-folder grants are migrated to direct per-user grants so upgrades preserve access. New shared folders wait for explicit per-user assignment.

## Security properties

- deny by default for non-administrator accounts;
- administrator is always full access;
- access changes affect existing sessions immediately because effective authorization is resolved from current state on authentication;
- disabling a user invalidates effective session use because disabled users cannot resolve an authenticated session;
- resource scopes do not imply global access;
- one user's resource grants never grant another user's access;
- password hashes and credentials are not exposed by the access APIs;
- access mutations are audited.

## Consequences

The Users page becomes the central control plane for human access across Home-AI. Modules no longer need their own account models and should depend on the Core actor/permission contract.

Profile names remain convenient household concepts, while permissions remain the actual enforcement mechanism.

A future module can add permissions to the central catalog and add resource discovery without changing the user identity model.

## Deferred

- deny/negative grants;
- wildcard resource scopes;
- temporary access expiry for guests;
- time-of-day / parental schedules;
- room-to-device implicit inheritance;
- camera/NVR/AI resource catalogs;
- password reset/change administration;
- invitations and remote account enrollment.
