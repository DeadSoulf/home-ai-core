# Modules

Home-AI functional extensions are independently distributed Docker modules.

The active module contract is implemented in:

- `internal/modules/` — manifest validation, dependency planning, registry and Docker helper client;
- `schemas/module-manifest-v2.schema.json` — canonical Manifest v2 schema;
- `schemas/module-repository-v1.schema.json` — repository/catalog foundation;
- `docs/sdk/module-sdk-v2.md` — module authoring and runtime contract.

Core contains no product-specific module implementation.

A module owns its business logic and persistent state. Core owns validation, authorization, jobs, audit, registry metadata and orchestration through the privileged helper.

Current runtime rules:

- Docker only;
- immutable image reference pinned with `@sha256`;
- dedicated `home-ai-modules` network;
- persistent data under `/var/lib/home-ai-core/modules/<module-id>`;
- no Docker socket inside Core or module containers;
- no host ports, devices or extra mounts in the initial v2 profile;
- install/start/stop/restart/remove run through the Job Engine.
