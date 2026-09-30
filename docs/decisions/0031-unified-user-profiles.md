# ADR-0031: Unified Core users with profile templates and explicit grants

## Status

Accepted for implementation, 2026-09-30.

## Context

Home-AI already has authenticated Core users, RBAC permissions and exact resource-scoped grants. The initial household slice, however, creates every non-owner account as a generic `member` and does not expose permission editing in the Web UI.

The product needs one identity per person across the whole Home-AI Core. A person may be an administrator, parent, child, friend or guest, while the owner must be able to customize exact capabilities and resource access without creating parallel user systems for NAS, NVR, Smart Home or AI.

## Decision

A Core user remains the canonical human identity for every Home-AI subsystem.

Home-AI provides profile templates:

- `administrator` — full access;
- `parent` — broad read-oriented household defaults;
- `child` — basic local access defaults;
- `friend` — sign-in plus explicitly granted access;
- `guest` — sign-in plus explicitly granted access;
- legacy `member` remains readable/editable for migration compatibility.

Profiles are defaults, not the final authorization boundary. For non-administrators an administrator may explicitly enable or disable individual global permissions for that user.

The bootstrap `owner` is presented as an Administrator profile but remains a protected root identity: it cannot be disabled or demoted. Additional `administrator` users receive every permission dynamically from the permission catalog, including permissions introduced by future migrations/modules.

Resource access continues to use the existing exact scoped-grant model. The first management UI exposes NAS `file_folder` resources with separate `files.read` and `files.write` grants. Future room, device and camera resources will use the same mechanism rather than new user databases.

Shared NAS folder semantics change from "all legacy members automatically" to explicit user access. Migration 014 converts any existing legacy role-level shared-folder grants into direct per-user grants before removing the role-wide grants, preserving existing access.

SMB users remain projections of Core users. SMB transport credentials stay separate from the Home-AI login password and do not create a second identity authority.

## Authorization model

Effective global permissions are:

1. all permissions for `owner` and `administrator`;
2. otherwise role-template permissions;
3. plus explicit per-user allows;
4. minus explicit per-user denies.

Effective resource permissions remain the union of role-scoped and direct user-scoped grants. New shared file folders are not role-granted automatically.

All authorization continues to be recalculated when a session is authenticated, so permission changes take effect without requiring a new account or a parallel identity.

## Consequences

The Users page becomes the central access-control surface for human accounts. Modules can register new permissions and later expose their resource catalogs through the same profile editor.

Profile names describe useful household defaults but do not silently expand authorization beyond the effective permission list.

## Deferred

- time windows / parental schedules;
- room/device/camera resource catalogs;
- approval-required operations;
- delegated administration subsets;
- group membership beyond the supplied household templates;
- passwordless/passkey authentication.
