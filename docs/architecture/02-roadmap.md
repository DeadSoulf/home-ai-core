# Home-AI Development Roadmap

## F0 — Core Foundation — complete

Foundation includes:

- Core daemon/API;
- node identity;
- SQLite state/migrations;
- authentication, sessions and RBAC;
- CSRF and audit;
- jobs and durable events;
- realtime transport;
- Module Registry / SDK;
- system and hardware discovery;
- storage/network foundation;
- privileged helper;
- Web UI shell;
- Core update/rollback;
- Debian installer;
- Windows file client;
- CI/release pipelines.

## F1 — Core cleanup — active

- remove embedded product implementations;
- remove product-specific API/Web/config/state;
- remove product-only host dependencies;
- preserve only generic Module Registry/SDK contracts;
- clean existing installations during upgrade.

## F2 — Docker Module Runtime

- install Docker on supported Debian hosts;
- detect `host.docker`;
- privileged helper operations for image/container lifecycle;
- no Docker socket access from Core;
- dedicated network and persistent data layout;
- health/status/log collection;
- resource/mount/device policy.

## F3 — Module Manifest v2

- container image and immutable digest;
- supported architectures;
- Core compatibility;
- declared capabilities;
- API/UI contribution metadata;
- volumes/mounts;
- network/ports;
- device requirements;
- health checks;
- upgrade/rollback metadata.

## F4 — Module lifecycle

- discover;
- install;
- start/enable;
- stop/disable;
- restart;
- update;
- rollback where supported;
- remove;
- logs/status.

## F5 — First external module

Create the first real functional module in its own repository and Docker image. Its code and domain database must not be copied back into Core.

## F6 — Distribution hardening

- signed module metadata;
- image provenance and digest pinning;
- compatibility checks;
- permission/capability review;
- backup/restore contracts;
- failure recovery;
- release channels.

## Rule

New product functionality is implemented as a module only after the generic Docker runtime is stable.
