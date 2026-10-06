# Security API v1

## Setup status

```text
GET /api/v1/security/setup-status
```

Unauthenticated. Reports only whether the Core has an initialized user database.

## First-owner bootstrap

```text
POST /api/v1/security/bootstrap
```

The peer must be localhost or a private local-network address. The endpoint is usable only while the user database is uninitialized.

Request:

```json
{
  "username": "owner",
  "display_name": "Home Owner",
  "password": "a long passphrase",
  "session_mode": "cookie"
}
```

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

Authentication does not make plaintext HTTP safe for untrusted networks. The Core listens on the local network by default for first-run convenience; do not expose port 8080 directly to the public Internet. Use the planned secure networking/TLS gateway for untrusted remote access.


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
