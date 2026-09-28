# ADR-0001: Repository Layout

- Status: Accepted
- Date: 2026-09-28

## Context

Home-AI-Core is being restarted to avoid coupling the platform core to individual server features.

The repository needs visible boundaries between trusted Core code, Web UI, SDK/contracts, optional Modules, Apps and distribution tooling.

## Decision

Use the following top-level ownership model:

```text
home-ai-core/
├── core/
├── web/
├── sdk/
├── schemas/
├── modules/
├── apps/
├── packaging/
├── scripts/
├── tests/
└── docs/
```

`core/` contains only platform responsibilities.

`modules/` contains optional host capabilities.

`apps/` contains user-facing services built on module capabilities.

`sdk/` and `schemas/` define stable extension contracts.

## Consequences

- Feature ownership is visible in the filesystem.
- The Core can be tested independently from optional capabilities.
- Modules can evolve independently behind versioned contracts.
- Cross-directory shortcuts that bypass public contracts are considered architectural violations.
