# Core State and Migration Engine

The initial Core implementation uses SQLite for local control-plane metadata.

## Filesystem layout

```text
/var/lib/home-ai-core/
├── core.db
├── core.db-wal
├── core.db-shm
└── identity/
    └── node-id
```

The database is not a storage location for user photos, videos, camera recordings, model files, container volumes or VM disks.

## Migration model

Migrations are embedded into the Core binary from:

```text
internal/state/migrations/
```

Naming convention:

```text
001_nodes.sql
002_...
003_...
```

Each migration version is applied once and recorded in `schema_migrations`.

A migration and its registry record are committed in the same SQLite transaction.

Core startup stops on migration failure instead of starting against a partially upgraded schema.

## SQLite configuration

The initial implementation uses:

- foreign keys enabled
- 5 second busy timeout
- WAL journal mode
- one database/sql connection

The single-connection limit is an intentional starting point for the control-plane database. It can be relaxed only after concurrency tests justify the change.

## Node record

On startup, the stable file-based node identity is registered in the database.

The first schema stores:

- node ID
- hostname
- creation timestamp
- last-seen timestamp

This is the first step toward future multi-node resource ownership without making the initial SQLite database a cluster database.
