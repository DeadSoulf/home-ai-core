# Home-AI-Core Web UI

The Web UI is a TypeScript/React single-page application.

## Navigation

Home shows essential server status and recent activity. The sidebar groups System under Server, Files and Modules under Services, and Users, Operations and Security under Management. Entries and management controls follow the signed-in user's permissions.

System uses four sections: Equipment, Storage, Network and Updates. Direct links use `/system#equipment`, `/system#storage`, `/system#network` and `/system#updates`; the update notification opens Updates directly. Technical identifiers and diagnostic metadata are available in expandable details.

On small screens, the menu opens as a drawer with keyboard focus containment, Escape dismissal and focus restoration.

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

First-owner bootstrap is intentionally localhost-only. Remote onboarding belongs to the later secure-networking work rather than the current Web UI foundation.
