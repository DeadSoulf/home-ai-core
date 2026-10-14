# Web UI audit corrections — PR #180

Base: `1e380f799fdc871465c9fc804151a1981f1710ba`, Core `0.1.173-dev`.
Scope: all 16 findings in the 0.1.173 Web UI audit. HTTPS port 82, trust-root
verification, first-owner proof, RBAC/CSRF and the different-image upgrade
safety gate are unchanged. No production deployment or Figma-file modification.

## Finding → correction → regression

| ID | Correction | Regression coverage |
|---|---|---|
| UI-01 | Sequential, cancellable reads; refreshes coalesce without discarding successful slow responses. Poll after completion; retain last data/error/timestamp; manual retry and slow notice. | resourcePoller unit tests; 2.6-second System replies, no overlap, retained data on 503. |
| UI-02 | Catalog and manual install remain available when Docker is absent. Explicit label explains helper preparation; the existing safe backend installation path prepares runtime. | First install with no Docker, queued/running/completed job. |
| UI-03 | Strict SemVer precedence, prerelease and build metadata handling. Older versions are not upgrades; newer/different images display a blocked-upgrade explanation and cannot replace the working module. | SemVer units and older/newer/same-version different-digest browser cases. |
| UI-04 | `registered` is an incomplete install. Retry uses the exact stored manifest, not a changed catalog entry. | Browser case with pending 1.2.0 and catalog 9.0.0; submitted JSON equals stored manifest. |
| UI-05 | Declared module routes render an honest protected-interface placeholder, not Dashboard. Direct paths/history/nested paths are recognized. Navigation failure has retry. | Menu, direct navigation, history and navigation-error tests; navigation unit tests. |
| UI-06 | Clear local auth/CSRF only after successful logout or authoritative 401. Failure leaves session visible and offers retry; repeated clicks are guarded. | Logout 503 then 204, preserved CSRF until confirmation. |
| UI-07 | Module jobs retain busy state, display progress/result and link to a selected operation. Polling works without events permission/WebSocket. Manager-only module job views avoid exposing arbitrary jobs/manifests. Backend serializes enqueue checks and rejects concurrent conflicts. | Browser pending/reopen/monitor-failure tests; concurrent Go enqueue and permission tests. |
| UI-08 | Accessible confirmation names the module, warns about stopping dependencies, and states ordinary removal preserves data. | No DELETE before confirmation, Escape cancels, focus restored, one confirmed request. |
| UI-09 | Cancel supported jobs with warning that cancellation is not rollback. Display acknowledged cancellation immediately; specific module job labels and selected-row focus. | Browser cancel and unit cancellation policy; backend remains authoritative. |
| UI-10 | Startup unavailable/network/server failure differs from 401/login and initial owner setup. Explicit connection retry. | setup-status 503 and me 503; recovery after retry. |
| UI-11 | Known API error codes and job stages localized in RU/EN. Technical details retain raw messages/request IDs. | Translation parity, invalid credentials and module progress browser checks. |
| UI-12 | Required fields, backend-aligned username and UTF-8 password rules, confirmation/show password, per-field messages and preservation on failure. | Auth units and login/first-owner browser validation. |
| UI-13 | Manifest label, described errors, alert/status/progress roles, accessible modal and focus. Existing mobile menu Escape/inert/focus behavior preserved. | DOM role/name checks and keyboard tests; not a complete screen-reader certification. |
| UI-14 | Read-only helper inspection distinguishes Docker presence, active service, responding engine and ready network. Socket capability is not readiness proof. | Runtime interpretation and endpoint Go tests; degraded engine/catalog browser tests. |
| UI-15 | Full-width themed JSON input with focus/error styling; manual Manifest moved to collapsed advanced settings. | CSS/DOM widths and screenshots on desktop/mobile. |
| UI-16 | Responsive card layouts, button gaps and separated destructive actions; runtime text no longer squeezed into definition-list columns. | Modules/Auth/Jobs/Users widths 320, 360, 390, 768, 1440. |

## Read-only API additions

- `GET /api/v1/modules/runtime`: `modules.read`, 22-second request context,
  helper `docker.inspect`; no Docker installation or container mutation.
- `GET /api/v1/modules/operations`: `modules.manage`, active module jobs only.
- `GET /api/v1/modules/operations/{jobID}`: `modules.manage`, module jobs only,
  including completed jobs for outcome tracking. Stored raw manifests are omitted.
- General `/jobs` endpoints retain their existing `jobs.read` / `jobs.cancel`
  requirements. New operation-submission checks remain behind existing
  `modules.manage` and CSRF boundaries. Incomplete/failed queue inspection fails
  closed rather than allowing a possibly conflicting duplicate operation.

## Verification

- Node 24 locked dependencies: TypeScript check, 55 Vitest tests and production
  Vite build passed in the isolated development environment.
- Full Go 1.27 tests and vet passed; race tests for changed API/modules passed.
- `web/tests/browser_regressions.py` has 16 grouped browser regressions covering
  all audited defects and permission/recovery/responsive cases. It uses the
  actual built bundle and synthetic API data, never a production server.
- Local restricted Chromium was run in explicit offline mode (network fixture
  binding and virtual history); this does **not** prove deployed TLS or native
  origin/history. The dedicated `Web UI browser regressions` CI workflow runs
  the same checks with real browser URLs/history and intercepted HTTPS fixtures.
- CI must pass on the final PR head before merge. Run results are linked in PR
  #180; this document is a reproducible test inventory, not a deployment claim.

Re-run:

```sh
(cd web && npm ci --ignore-scripts --no-audit --no-fund && npm run typecheck && npm test && npm run build)
python -m pip install playwright==1.57.0
python -m playwright install --with-deps chromium
UI_EVIDENCE_DIR=./ui-evidence python web/tests/browser_regressions.py
go test ./...
go vet ./...
go test -race ./internal/api ./internal/modules
```

## Deliberate remaining boundaries

No Figma file was supplied, so pixel parity with a design is not asserted.
Module application UI routing is explicitly not yet connected; the fallback
now tells the truth. No new module proxy, arbitrary external iframe, automatic
upgrade/data migration or weaker Docker permissions were introduced. Full
transactional different-image upgrades remain issue #177. Manual Manifest
installation is advanced/admin-only, not removed from the backend contract.

`VERSION` is not bumped for this UI correction, so no unsigned release is
triggered. The earlier signing-key/publication and one-time HTTPS migration
requirements remain in `SECURITY_PORT82_MIGRATION.md`. Real-server acceptance,
certificate trust and original data checks remain separate administrative work.
