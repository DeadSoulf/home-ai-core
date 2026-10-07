# Module SDK v2 Acceptance Criteria

## Manifest contract

- [x] Manifest v2 JSON Schema exists;
- [x] unknown fields are rejected;
- [x] stable module IDs and semantic versions are validated;
- [x] Core compatibility constraints are validated;
- [x] dependency/conflict planning remains available;
- [x] supported architectures are declarative;
- [x] Docker runtime is mandatory;
- [x] image reference must be immutable `@sha256`;
- [x] `host.docker` is mandatory for Docker modules;
- [x] arbitrary shell commands and host package installation are not part of the manifest.

## Runtime isolation

- [x] Core has no Docker socket access;
- [x] modules have no Docker socket access;
- [x] container lifecycle crosses the privileged helper boundary;
- [x] helper refuses unmanaged container collisions;
- [x] container root filesystem is read-only;
- [x] all Linux capabilities are dropped;
- [x] `no-new-privileges` is enabled;
- [x] module process runs as the unprivileged Home-AI service UID/GID;
- [x] only module `/data` and bounded tmpfs are writable;
- [x] all modules join the dedicated `home-ai-modules` network.

## Lifecycle

- [x] install is a persistent Job Engine operation;
- [x] start/enable is a persistent job;
- [x] stop/disable is a persistent job;
- [x] restart is a persistent job;
- [x] remove is a persistent job;
- [x] persistent data is preserved on remove;
- [x] lifecycle mutations require `modules.manage`;
- [x] cookie mutations require CSRF protection;
- [x] queued lifecycle actions are audited.

## Registry/API/UI

- [x] module manifests and status persist in SQLite;
- [x] module list/detail/capability APIs exist;
- [x] Manifest v2 installation API exists;
- [x] Web UI accepts a trusted Manifest v2;
- [x] Web UI exposes start/stop/restart/remove controls;
- [x] declarative navigation remains supported;
- [x] manifest-provided executable frontend code is not supported.

## Remaining hardening

- [ ] signed Docker module catalog;
- [ ] install-plan preview enforced before mutation;
- [ ] dependency/conflict enforcement during installation;
- [ ] health reconciliation;
- [ ] module log access;
- [ ] explicit module update operation;
- [ ] explicit persistent-data purge;
- [ ] physical-node lifecycle acceptance tests.
