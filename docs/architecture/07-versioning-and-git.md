# Versioning and Git

## Versioning

Home-AI-Core uses semantic versioning.

During initial development:

- `0.1.x` — Core foundation
- `0.2.x` — Module SDK
- `0.3.x` — Web UI foundation
- later minor releases add major platform capabilities

The current development version is stored in the root `VERSION` file.

## Branches

- `main` — reviewed and accepted project baseline
- `develop` — integration branch for the current phase
- `feature/<name>` — isolated feature work when needed
- `fix/<name>` — fixes
- `release/<version>` — release stabilization when required
- `archive/<name>` — preserved historical snapshots

## Merge rule

No feature is merged into `main` until:

- its contract is documented
- tests for its scope pass
- security implications are reviewed
- required third-party licenses are recorded
- relevant documentation is updated

## Commits

Use concise conventional prefixes where practical:

- `feat:`
- `fix:`
- `docs:`
- `test:`
- `refactor:`
- `chore:`
