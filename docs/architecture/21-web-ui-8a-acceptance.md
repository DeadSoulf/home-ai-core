# Web UI Phase 8A Acceptance Criteria

## Authentication

- [x] setup-status determines first-run vs login flow
- [x] first-owner form uses the existing bootstrap-token API
- [x] first-owner local-only security remains enforced
- [x] login uses cookie sessions
- [x] bearer tokens are not stored by the Web UI
- [x] logout uses the existing CSRF contract

## Shell and pages

- [x] responsive application shell
- [x] Dashboard
- [x] System
- [x] Modules
- [x] Jobs
- [x] Audit
- [x] unknown SPA routes return to Dashboard
- [x] UI does not display unsupported fake management actions

## Realtime

- [x] same-origin WebSocket connection
- [x] relevant topic subscriptions
- [x] visible connection state
- [x] reconnect with bounded exponential delay
- [x] non-Core events trigger REST refresh

## Production runtime

- [x] Web UI compiles to static files
- [x] Node.js is build-time only
- [x] Core can serve a configured Web UI directory
- [x] API routes cannot be shadowed by SPA routing
- [x] missing Web bundle preserves API-only behaviour
- [x] Debian runtime defines the Web UI install directory
- [x] UI responses set a restrictive Content-Security-Policy
- [x] UI responses deny framing and referrer leakage
- [x] static assets use MIME sniffing protection

## Validation

- [x] TypeScript strict mode
- [x] API client unit tests
- [x] Go Web UI handler tests
- [x] production Vite build
- [x] committed npm dependency lock
- [x] CI installs Web dependencies with `npm ci`
- [x] existing Go unit/vet/smoke/cross-build checks remain required

## Deferred to Phase 8.5 / later

- production LAN/TLS onboarding
- signed Debian packaging
- one-command installer
- user administration UI
- settings mutation UI
- Module Store install/update/remove
