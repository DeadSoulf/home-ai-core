# ADR-0004: Core Configuration and State Storage

- Status: Accepted
- Date: 2026-09-28

## Context

Home-AI-Core needs durable control-plane state for:

- node identity
- users and device registrations
- roles, permissions and grants
- installed module metadata
- jobs and their progress/result metadata
- audit records
- resource inventory metadata
- update state
- application/module registry metadata

At the same time, Home-AI-Core will manage large user data sets such as photos, videos, camera recordings, backups and VM images. Those workloads must not be stored inside the Core control database.

The platform also needs a clean distinction between administrator bootstrap configuration and mutable runtime state.

## Decision

### Core state database: SQLite

The single-node Core uses SQLite as its durable control-plane database.

Default location:

```text
/var/lib/home-ai-core/core.db
```

The database is owned by the dedicated Home-AI-Core service account and is not exposed directly to modules or clients.

SQLite is used for control metadata only.

It does not store:

- user file contents
- photo/video binaries
- camera recordings
- VM disks
- container volumes
- model files
- backup payloads

Those belong to storage/modules and are referenced by stable resource identifiers and metadata.

### Database access

Only the Core owns the Core database schema.

Modules do not create arbitrary tables inside `core.db`.

A module that needs durable private state receives an isolated state directory and may use its own appropriate storage engine.

Conceptual layout:

```text
/var/lib/home-ai-core/
├── core.db
├── identity/
├── modules/
│   ├── containers/
│   ├── storage/
│   └── ...
└── cache/
```

### SQLite mode and migrations

The implementation should use:

- transactional writes
- foreign-key enforcement
- WAL mode where supported and validated for the deployment filesystem
- explicit schema migrations
- migration version tracking
- backups before destructive schema migrations

Database migrations are shipped with the Core executable/source tree and are applied through a controlled migration layer.

Startup must fail safely rather than partially applying an incompatible migration.

### Static/bootstrap configuration

Administrator-controlled bootstrap configuration lives under:

```text
/etc/home-ai-core/
```

This layer is intentionally small.

It is suitable for values such as:

- listen addresses
- explicitly selected data/state locations
- logging overrides
- recovery/emergency options
- deployment-specific bootstrap settings

Mutable product configuration should normally be managed through the Core API and stored in the control database rather than repeatedly rewriting configuration files.

### Runtime data

Ephemeral runtime files live under:

```text
/run/home-ai-core/
```

Examples:

- Unix sockets
- PID/runtime markers where required
- short-lived IPC state

Temporary/cache data lives in an appropriate cache directory and is safe to recreate.

### Secrets

Secrets must not be stored in:

- source control
- normal logs
- generic module manifests
- plaintext administrator documentation
- public API responses

The Core exposes a secret-store abstraction.

For the initial implementation, secret material may be stored in dedicated permission-restricted files/state owned by the relevant service account, with database rows storing references and metadata rather than duplicating secret values.

The architecture must allow a later encrypted or platform-backed secret store without changing module-facing contracts.

### Logs and audit

Normal operational logs go to the system logging path (initially journald through stdout/stderr/systemd integration).

Audit records are structured Core state and may be stored in the Core database with retention/export policies.

Large raw logs are not accumulated indefinitely inside SQLite.

### Backups

A valid Core backup contains at minimum:

- a consistent snapshot of `core.db`
- node/platform configuration required for recovery
- secret material according to the backup policy
- module registry/configuration metadata

Backups must use SQLite-consistent backup/snapshot mechanisms rather than copying a live database file blindly.

## Multi-node consequence

SQLite is a local node/control-plane persistence choice, not a distributed consensus database.

Future multi-node operation must not attempt to share `core.db` over NFS/SMB or replicate the SQLite file directly.

Cluster-wide state will be introduced behind a separate documented coordination/consensus layer.

Local node state remains local where appropriate.

This distinction allows single-node development to stay simple without preventing a later distributed control plane.

## Resource identity

Persistent records use stable generated identifiers rather than filesystem paths as primary identity.

A resource record may reference:

- node ID
- resource ID
- type
- owner/tenant where applicable
- provider/module
- external/storage location

Paths and device names are mutable attributes, not durable global identity.

## Consequences

### Positive

- Very small operational footprint for a home server.
- Transactional local state without deploying a separate database service.
- Straightforward backup and migration story.
- Modules cannot corrupt Core schema through direct database ownership.
- Large home data remains in storage designed for large objects rather than in the control database.
- Multi-node architecture is not falsely coupled to a shared-filesystem SQLite design.

### Negative

- Cluster-wide strongly consistent state needs a separate future design.
- Write-heavy telemetry/event history should not be pushed indiscriminately into SQLite.
- The project must maintain careful schema migrations.

## Rejected alternatives

### Store everything in YAML/JSON files

Rejected for mutable Core state because users, permissions, jobs, audit records and concurrent updates require transactions, indexing and consistent migrations.

### PostgreSQL as a mandatory Core dependency

Rejected for the initial home-server control plane because it increases installation and operational complexity for a service whose local metadata fits an embedded transactional database.

PostgreSQL remains an acceptable dependency for individual Apps/Modules that need it.

### Shared SQLite database across nodes

Rejected. A database file is not the multi-node coordination protocol.

### Put module-private tables directly in the Core database

Rejected because it couples module lifecycle and migrations to the Core schema and makes safe removal/upgrades harder.
