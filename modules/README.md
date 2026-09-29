# Modules

This directory is reserved for independently installable Home-AI-Core capability modules.

The module contract itself is implemented in:

- `internal/modules/`
- `schemas/module-manifest-v1.schema.json`
- `schemas/module-repository-v1.schema.json`
- `docs/sdk/module-sdk-v1.md`

Current Core code contains the module registry, dependency/conflict planner, capability discovery and signed repository verification foundation.

Higher-level capabilities such as NAS, storage pools, backup, containers, virtualization, NVR, AI and VPN should be implemented behind these module boundaries rather than embedded directly into unrelated Core code.
