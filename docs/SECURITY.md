# Home AI Core — Security and Users

## Central identity database

Home AI Core now uses one central local user/security database for the whole server:

```text
runtime/security/security.db
```

The database is SQLite and is used by Web/API today and is the identity/permission source for future Files, Cameras, Smart Home, AI, Automation and Hypervisor modules.

SQLite is embedded as a library. No external database server is required.

At startup Security Core enables:

```text
foreign_keys=ON
journal_mode=WAL
synchronous=NORMAL
busy_timeout=5000 ms
```

The security directory is restricted to the service account. The main database is set to mode `0600`.

## Legacy migration

Existing installations are migrated automatically from:

```text
runtime/security/users.db
runtime/security/audit.log
```

The legacy password records are imported without changing the PBKDF2 salt/hash, so existing passwords continue to work.

The legacy user parser is fail-closed: malformed user records stop migration instead of being silently ignored.

After a successful import the old files are archived with the suffix:

```text
.migrated
```

Future SQLite schema migrations use `PRAGMA user_version` and can create a pre-migration backup.

## Passwords

Passwords remain protected with:

- PBKDF2-HMAC-SHA256
- random per-user 16-byte salt
- 310,000 iterations
- 32-byte derived hash
- constant-time comparison through OpenSSL

Unknown usernames perform a dummy PBKDF2 calculation to reduce username-enumeration timing differences.

Password reset invalidates all active sessions of that user.

## Roles

Built-in roles provide default permissions:

### viewer

Read/use access for normal server functions:

- files.read
- cameras.view
- smart_home.view
- ai.use
- system.view
- storage.view
- network.view
- automation.view

### operator

Includes Viewer defaults plus operational actions:

- files.write
- cameras.manage
- smart_home.control
- automation.manage

### admin

All known permissions.

## Granular permissions

Every user can override role defaults with one of:

```text
inherit
allow
deny
```

Current permission catalog:

```text
files.read
files.write
files.manage

cameras.view
cameras.manage

smart_home.view
smart_home.control
smart_home.manage

ai.use
ai.manage

users.view
users.manage

system.view
system.manage

storage.view
storage.manage

network.view
network.manage

hypervisor.view
hypervisor.manage

automation.view
automation.manage
```

Overrides are enforced by backend API checks. Hiding a Web button is not considered an authorization boundary.

The last enabled administrator cannot be deleted, disabled or demoted. `users.manage` also cannot be explicitly denied for the last enabled administrator.

## Sessions

Sessions are stored in the central database so they survive a normal service restart.

The browser receives a cryptographically random session token. Only SHA-256 of that token is stored in the database.

Current properties:

- 8-hour expiry
- HttpOnly cookie
- SameSite=Strict
- persistent session metadata
- created time
- last-seen time
- per-session revocation
- revoke all sessions for a user
- disabled accounts immediately lose valid access

Last-seen is updated periodically rather than on every HTTP request.

## Login throttling

Login throttling is applied at two independent levels:

- account key: 5 failed attempts lock that username for 60 seconds
- source key: 20 failed attempts from one peer IP across any usernames lock that source for 5 minutes

The source address is taken from the accepted TCP connection with `getpeername()`. Home AI Core
does **not** trust `X-Forwarded-For` or similar headers by default, because an untrusted client
could spoof them and bypass source throttling. A future reverse-proxy mode must explicitly define
trusted proxy addresses before forwarded client IPs can be accepted.

Failure counters are runtime-only and stale unlocked counters expire automatically. A successful
login clears the account-specific failure state but does not erase accumulated failures for the
entire source address, so possession of one valid account cannot reset the source-wide limiter.

## Security audit

Audit events are stored in the same central database.

The Users page can filter and page through the audit history.

Examples include:

- login success/failure
- logout
- user creation/update/delete
- password reset
- permission override
- session revocation
- configuration and administrative actions

## Users Web section

The `/users` page implements:

- user list
- create user
- role change
- enable/disable account
- password reset
- effective permission display
- allow/deny/inherit permission overrides
- active session list
- revoke one session
- revoke all sessions for a user
- security audit viewer with filters and pagination

Viewing requires:

```text
users.view
```

Mutation/session/audit administration requires:

```text
users.manage
```

## Protected subsystem permissions

Existing server APIs now use domain permissions instead of only checking the `admin` role.

Examples:

```text
system.manage    configuration and server update actions
storage.manage   disk actions
network.manage   network/VPN actions
ai.manage        GPU/AI administration
users.manage     user administration
```

Read-only subsystem endpoints use corresponding `.view` permissions.

## Runtime configuration

Development defaults:

```text
security.database_file=runtime/security/security.db

# one-time migration sources
security.users_file=runtime/security/users.db
security.audit_file=runtime/security/audit.log
```

Runtime security data must never be committed to Git.

## Dependency

Building the SQLite-backed Security Core requires the SQLite development library on Debian:

```bash
sudo apt install libsqlite3-dev
```

This is a build-time/library dependency, not a separate database service.

## HTTPS/TLS

The Web Core can serve HTTPS directly with OpenSSL. TLS is deliberately disabled by default so
an update cannot lock an existing installation out before a certificate is prepared.

Configuration:

```text
web.tls_enabled=false
web.tls_certificate=runtime/tls/server.crt
web.tls_private_key=runtime/tls/server.key
```

Starting with 0.0.52 these values can also be changed from the Network page by a user with
`network.manage`. Certificate and key paths are constrained to `runtime/tls/`, and changes
take effect after a Core restart.

When enabled, startup fails closed if the certificate is missing, the private key is invalid,
or the key does not match the certificate. TLS 1.2 or newer is required. HTTPS responses add
HSTS and authenticated session cookies receive the `Secure` attribute.

A local self-signed certificate can be generated without root access:

```bash
bash scripts/setup-tls.sh home-ai-core.local
```

A self-signed certificate must be trusted on client devices to remove browser warnings. For
remote/production administration, use a certificate issued by a trusted internal or public CA.
Do not expose an HTTP-only management port directly to the public Internet.

Cluster heartbeat traffic remains a separate concern: keep it on a trusted private network or
WireGuard as documented by Cluster Core. Enabling Web TLS does not automatically change worker
heartbeat transport.

## CSRF protection

Authenticated state-changing requests use a session-bound CSRF token in addition to normal
permission checks. The token is derived from the cryptographically random session secret and
changes automatically with the session; it is not stored as an additional database secret.

Web/API mutation rules:

- authenticated API `POST` / `PUT` / `PATCH` / `DELETE` requests require
  `X-HomeAI-Request: 1`
- the same API requests require `X-HomeAI-CSRF` matching the current session
- normal Web UI forms use the same session-bound token as a hidden `_csrf` value
- the Web UI automatically adds both protection headers to same-origin mutation requests
- legacy settings forms targeting `/api/config` are intercepted and submitted through the same protected fetch path
- GET/HEAD/OPTIONS remain read-only and do not require a CSRF token
- Cluster worker heartbeats are machine-to-machine traffic and keep their independent
  `X-HomeAI-Cluster-Token` authentication path

The authenticated `GET /api/session` response includes the current `csrf_token` for
non-browser API clients. A CSRF token from another session is rejected.

## Planned hardening

Source/IP-aware login throttling is implemented as of 0.0.43.

Remaining hardening:

- trusted reverse-proxy mode with an explicit proxy allowlist before forwarded client IPs are accepted
- optional MFA
- encrypted secrets store
- signed update/module verification
- AI sandbox permissions
- richer audit retention/export policies
