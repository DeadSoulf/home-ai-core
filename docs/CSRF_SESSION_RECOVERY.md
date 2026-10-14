# CSRF session recovery after HTTPS/port migration

## Confirmed failure path in 0.1.173-dev

A browser can send a valid `home_ai_session` cookie while its current origin
has no matching `home-ai-core.csrf` entry. `/auth/me` previously returned only
the actor; App treated that as complete browser authentication. A console
ticket or logout then returned 403 `csrf_required`, leaving the user inside an
interface with no usable sign-out path. A missing token, token from another
session and browser storage restrictions need distinct handling, not removal
of CSRF protection. The user's repeated console error proves server rejection;
it does not, by itself, prove which browser condition caused it.

## Correction

- GET `/api/v1/auth/me` reports read-only `csrf_valid` for the supplied header
  and authenticated cookie. It returns no CSRF/session secret, never changes
  database session hashes and uses `Cache-Control: no-store`. Bearer clients
  keep their existing contract and do not require cookie CSRF.
- The Web client sends its current synchronizer token when restoring a session.
  Missing/invalid readiness goes to an explicit reauthentication form instead
  of silently rendering the privileged UI. Old server responses missing the
  field fail closed; deploy the Core and Web bundle together.
- After a successful login/bootstrap, the client confirms the actual cookie,
  CSRF pair and actor before reporting authenticated state. Rejected cookies,
  stale headers or a different account cannot silently enter the app.
- A 403 `csrf_required` from an attempted protected request opens that same
  form. The previous server session is NOT claimed to be revoked, and the
  failed mutation is never automatically replayed after login. The existing
  account is reused; no owner reset or database deletion is involved.
- If persistent browser storage is unavailable, a freshly issued CSRF token
  is kept only in tab memory. A reload without persistent storage requires
  login again. Locale preference reads/writes also tolerate blocked storage,
  so the sign-in form remains renderable. No authentication cookie becomes
  accessible to JavaScript.
- A console ticket request is cancellable and time-bounded. Unmount, session
  recovery or disconnect invalidates its generation, so a late response cannot
  open a background shell or overwrite a later connection's state.

TLS, Secure/HttpOnly cookies, server-side permission checks, synchronizer-token
validation, one-time/session-bound console tickets, the helper boundary and
update signing are not weakened. Protocol and database schema are unchanged.

## Regression coverage

`internal/api/csrf_readiness_test.go` exercises real temporary SQLite sessions,
real login and console-ticket handlers: missing/stale/cross-session token,
fresh cookie-mode login, no secret disclosure and unchanged existing tokens.
It does not start a shell or use production data.

`web/src/api/csrf-session.test.ts` covers header transmission, restored session
validation, blocked/quota-limited storage, login confirmation, account mismatch,
no implicit retry and ignoring obsolete request errors.

`web/tests/csrf_regressions.py` runs the production Web bundle in real Chromium
with synthetic API sessions and a fake WebSocket (never a production shell).
It covers missing/stale migrated storage, a fresh private session, blocked
storage, rejected console/logout, explicit successful re-login, input delivery,
and navigation away while a ticket is pending. This complements, rather than
replaces, the Go handler checks; it is not a live-server acceptance result.

The existing 16 grouped Web UI regressions remain enabled. Actual run IDs and
results must be recorded in the PR before release. The published 173 binaries
must not be replaced: distribute this source change under a new signed version.
No real server is updated by writing this document.
