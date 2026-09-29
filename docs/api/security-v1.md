# Security API v1

## Setup status

```text
GET /api/v1/security/setup-status
```

Unauthenticated. Reports only whether the Core has an initialized user database.

## First-owner bootstrap

```text
POST /api/v1/security/bootstrap
X-Home-AI-Bootstrap-Token: <local one-time token>
```

The peer must be loopback.

Request:

```json
{
  "username": "owner",
  "display_name": "Home Owner",
  "password": "a long passphrase",
  "session_mode": "cookie"
}
```

The bootstrap token is stored at `/var/lib/home-ai-core/bootstrap-token` with mode `0600` until initialization succeeds.

## Login

```text
POST /api/v1/auth/login
```

Request:

```json
{
  "username": "owner",
  "password": "a long passphrase",
  "session_mode": "cookie"
}
```

`session_mode` may be:

- `cookie` — default; sets the `home_ai_session` HttpOnly SameSite=Strict cookie and returns a CSRF token
- `token` — returns an opaque bearer token for API/CLI use

## Current identity

```text
GET /api/v1/auth/me
```

Requires authentication and `security.self.read`.

## Logout

```text
POST /api/v1/auth/logout
```

Cookie-authenticated requests must include:

```text
X-CSRF-Token: <session csrf token>
```

Bearer-authenticated requests do not require the CSRF header.

## Audit

```text
GET /api/v1/audit?limit=100
```

Requires `audit.read`.

## Existing protected endpoints

- `GET /api/v1/system` requires `system.read`
- `GET /api/v1/events` requires `events.read`

`GET /health` remains unauthenticated for service supervision.

## Transport warning

Authentication does not make plaintext remote HTTP safe. The Core remains loopback-only by default until the secure networking/TLS gateway phase provides an approved remote-access path.


## User management

List users:

```text
GET /api/v1/security/users
```

Requires `security.users.read`.

Create a household member:

```text
POST /api/v1/security/users
```

Requires `security.users.manage`. Cookie-authenticated requests also require a valid `X-CSRF-Token`.

Request:

```json
{
  "username": "alice",
  "display_name": "Alice",
  "password": "a long passphrase"
}
```

New users receive the built-in `member` role. The role grants only:

- `security.self.read`;
- `security.sessions.manage`.

System, storage, update, module, audit and future Home-AI domain access are assigned explicitly rather than inherited automatically.
