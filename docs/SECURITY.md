# Home AI Core — Security

## Current security model

Version 0.0.5 introduces the first integrated Security Core.

Implemented foundations:

- local users
- PBKDF2-HMAC-SHA256 password hashing
- per-user random salts
- 310,000 PBKDF2 iterations
- cryptographically random session tokens
- 8-hour in-memory sessions
- roles: `admin`, `operator`, `viewer`
- first-run administrator setup
- login and logout
- protection of Web Core and API routes
- temporary login throttling after repeated failures
- audit log
- restrictive security headers
- HttpOnly / SameSite=Strict session cookie

## First startup

When no local users exist, browsing to the Web Core redirects to:

```text
/setup
```

The setup page creates the first administrator account.

After setup, unauthenticated requests are redirected to:

```text
/login
```

## Protected API

Authenticated users can access:

- `/api/status`
- `/api/system`
- `/api/session`

Administrative configuration endpoints require the `admin` role:

- `GET /api/config`
- `POST /api/config`

## Runtime security files

Development defaults:

```text
runtime/security/users.db
runtime/security/audit.log
```

The user database contains salted password hashes, never plaintext passwords.

The audit log records security and administrative events.

These runtime files must never be committed to Git.

## Current limitation: HTTP

The development Web Core currently uses plain HTTP.

Therefore 0.0.5 is suitable only for a trusted development/local network. Do not expose port 8080 directly to the Internet.

TLS/HTTPS support must be implemented before remote administrative access is considered production-ready.

## Planned hardening

Future security work includes:

- TLS / HTTPS
- CSRF tokens for state-changing Web actions
- user-management UI
- role-specific permissions per module/action
- session revocation and management
- stronger login throttling by account and source
- security event viewer
- optional multi-factor authentication
- signed module/update verification
- sandbox permissions for AI-generated code
- secrets storage separated from normal configuration
