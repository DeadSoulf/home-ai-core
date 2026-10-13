# Changelog

## Unreleased

### Core architecture reset

- Product-specific embedded runtimes have been removed from Home-AI-Core.
- Core is now defined as a trusted control plane rather than a bundle of product workloads.
- Module Registry and Module SDK remain as the foundation for independently distributed modules.
- Product modules will be delivered as separate Docker containers with independent versions and lifecycle.
- Core no longer carries module-specific runtime dependencies.
- Upgraded installations receive a cleanup migration that removes retired embedded product state.
- The next development stage is Docker Module Runtime.

Historical implementation notes for removed embedded prototypes are intentionally not retained in the active changelog.
