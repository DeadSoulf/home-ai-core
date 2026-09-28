# Third-Party Code Register

## Direct dependencies

| Component | Upstream project | Version | License | Modified | Home-AI-Core location |
|---|---|---:|---|---|---|
| WebSocket transport | github.com/coder/websocket | v1.8.15 | ISC | No | `internal/realtime` |
| Password hashing | golang.org/x/crypto/argon2 | v0.57.0 | BSD-3-Clause | No | `internal/security` |
| SQLite Go driver | modernc.org/sqlite | v1.59.0 | BSD-3-Clause | No | Go module dependency used by `internal/state` |

`github.com/coder/websocket` is used as a minimal WebSocket transport and has no runtime dependencies of its own.

`golang.org/x/crypto/argon2` provides the Argon2id implementation used for password hashing.

`modernc.org/sqlite` is a pure-Go SQLite driver/port and includes its own transitive third-party components under their respective upstream licenses. Dependency metadata is managed through Go modules.

## Architectural references

Architectural inspiration without source-code reuse is documented in `docs/architecture/03-third-party-code-policy.md`.

No source files from the previously reviewed server-management panels have been copied into the restarted Home-AI-Core codebase.
