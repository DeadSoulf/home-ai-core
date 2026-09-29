# ADR-0001: Repository Layout

- Status: Amended
- Original date: 2026-09-28
- Amended: 2026-09-29

## Context

The original repository skeleton reserved empty top-level directories for future Core, SDK, Apps and test implementations. The real implementation subsequently converged on standard Go and Web layouts, leaving several placeholder directories with no executable purpose.

Keeping empty architectural placeholders made the repository harder to read and implied that the active implementation lived somewhere other than it actually does.

## Decision

Use the implemented ownership model:

```text
home-ai-core/
├── cmd/                  executable entry points
├── internal/             trusted Core implementation
├── web/                  React/TypeScript Web UI
├── schemas/              external JSON contracts
├── modules/              future installable module workspace
├── packaging/            Debian/systemd assets
├── scripts/              build and validation scripts
├── docs/                 architecture, API and ADRs
└── .github/workflows/    CI, initial installer and update publishing
```

Go tests remain next to the packages they exercise. Web tests remain next to Web source. Module SDK contracts live under `internal/modules`, `schemas/` and `docs/sdk/`.

Empty `core/`, `sdk/`, `apps/` and `tests/` placeholder directories are not retained.

## Boundaries

- `cmd/home-ai-core` is the unprivileged network-facing control plane.
- `cmd/home-ai-core-updater` is the privileged helper.
- `internal/` owns platform implementation only.
- `web/` never executes privileged host commands directly.
- `modules/` is reserved for future independently installable capability implementations.
- High-level product features should use module contracts rather than bypassing Core boundaries.

## Consequences

- the filesystem now matches the actual implementation;
- dead scaffold is removed;
- tests follow normal Go/Web conventions;
- architecture boundaries remain explicit without placeholder directories.
