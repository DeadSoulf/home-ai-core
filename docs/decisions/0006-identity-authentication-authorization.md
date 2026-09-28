# ADR-0006: Identity, Authentication and Authorization

- Status: Accepted
- Date: 2026-09-28

## Context

Home-AI-Core controls personal data, cameras, smart-home devices, AI tools and eventually multiple physical servers. Authentication protects both private information and real-world actions.

The platform must later represent human users, mobile devices, modules, applications, cluster nodes and AI actors. Phase 5 implements human-user sessions first while preserving a common actor/capability model.

## Decision

### Actor model

Authenticated actions are attributed to actors. Initial actor type is `user`. Reserved future types are `device`, `module`, `app`, `node`, `ai` and `service`.

Authorization is deny-by-default.

### Password storage

Passwords use Argon2id through `golang.org/x/crypto/argon2`.

Initial parameters:

```text
memory:      19456 KiB
iterations:  2
parallelism: 1
salt:        16 random bytes
key:         32 bytes
```

Passwords are never logged or stored reversibly.

### Sessions

Interactive sessions use 256-bit random opaque tokens. Only SHA-256 hashes of session tokens are stored in SQLite. Initial lifetime is 24 hours.

Supported transports:

- browser: HttpOnly, SameSite=Strict cookie
- API/CLI: Authorization Bearer token

Raw bearer tokens are returned only when token mode is explicitly requested.

### CSRF

Cookie-authenticated state-changing requests require a separate `X-CSRF-Token`. Only its hash is stored server-side. Bearer-authenticated requests do not use ambient cookie authority and do not require this header.

### Bootstrap

When no users exist, Core creates `/var/lib/home-ai-core/bootstrap-token` with mode `0600`.

The first-owner bootstrap endpoint is loopback-only, requires that token, works only while no user exists, and removes the token after successful initialization.

### RBAC

Phase 5 creates users, roles, permissions, user-role bindings and role-permission bindings.

The initial `owner` role receives explicit current permissions:

- `system.read`
- `events.read`
- `security.self.read`
- `security.sessions.manage`
- `security.users.manage`
- `security.roles.manage`
- `audit.read`

Future permissions are granted deliberately by migrations rather than by an automatic wildcard.

### Audit

Bootstrap, login success/failure and logout create structured audit events carrying request/correlation identity and safe metadata. Passwords and all authentication secrets are excluded.

### Public endpoints

Unauthenticated endpoints are limited to:

- `GET /health`
- `GET /api/v1/security/setup-status`
- `POST /api/v1/security/bootstrap`
- `POST /api/v1/auth/login`

Existing system/realtime APIs become authenticated.

## Consequences

Authentication is available before network exposure grows, but remote plaintext HTTP is still not approved. TLS/gateway work remains a separate phase.
