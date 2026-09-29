# ADR-0015: Household User Foundation

- Status: Accepted
- Date: 2026-09-29

## Context

Home-AI is a multi-user household product. Different users must eventually receive different access to rooms, devices, cameras, files and AI tools.

The initial implementation only had the bootstrap owner.

## Decision

Add a built-in `member` role and explicit user-management APIs.

The existing `owner` role remains the administrative account and receives `security.users.read` in addition to the existing `security.users.manage`.

A newly created household user receives the `member` role. The role intentionally grants only:

- `security.self.read`;
- `security.sessions.manage`.

It receives no broad system, storage, module, update, audit or event permissions. Product-domain access is granted explicitly later through global or resource-scoped permissions.

The first user-management surface contains only listing users and creating a member user. Password hashes are never exposed.

## Consequences

The system can represent multiple human accounts without giving new users administrative authority by default.

NAS, Smart Home, Cameras and AI can build on stable user IDs and scoped permissions.
