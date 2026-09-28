# Home-AI-Core Web UI

The Web UI is a TypeScript/React single-page application.

## Development

Use Node.js 24 LTS.

```sh
cd web
npm install
npm run dev
```

Vite proxies `/api` and `/health` to a Core daemon listening on `127.0.0.1:8080`.

## Validation

```sh
npm run typecheck
npm test
npm run build
```

The production bundle is written to `web/dist`.

On Debian the bundle is installed under `/usr/share/home-ai-core/web` and is served by the Go Core. Node.js is not required on the production server.

First-owner bootstrap remains localhost-only until Phase 8.5 introduces the approved secure LAN/TLS onboarding flow.
