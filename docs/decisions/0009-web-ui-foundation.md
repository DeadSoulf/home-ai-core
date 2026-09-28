# ADR-0009: Web UI Foundation

- Status: Accepted
- Date: 2026-09-28

## Decision

Home-AI-Core Web UI is a same-origin single-page application built with TypeScript and React.

Initial toolchain:

- React 19.3
- TypeScript 7.0
- Vite 8.3
- Vitest 5
- Node.js 24 LTS for development/CI

Node.js is a build-time dependency only. The production server does not require a Node runtime.

The built SPA is installed under:

```text
/usr/share/home-ai-core/web
```

The Go Core serves that directory for non-API GET/HEAD requests. `/api/*` and `/health` always remain backend routes.

Phase 8A contains first-owner setup, login/logout, Dashboard, System, Modules, Jobs, Audit and realtime connection state.

Only backend-supported functionality is rendered. There are no fake install, storage, KVM or AI controls.

The browser uses the existing HttpOnly SameSite=Strict session cookie. The CSRF token returned at login/bootstrap is stored separately and is not an authentication credential. The UI never stores a bearer token.

The existing local-only first-owner bootstrap rule remains unchanged. Until Phase 8.5 provides an approved LAN/TLS onboarding flow, first-owner setup is performed through localhost, for example via an SSH tunnel.

The UI uses same-origin `/api/v1/events`; domain events trigger REST refresh while REST remains authoritative state.

The initial UI deliberately avoids client-side state frameworks, third-party component libraries, charting libraries, router libraries and icon packages.
