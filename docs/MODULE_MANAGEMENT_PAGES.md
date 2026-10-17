# Module management pages

The Core UI separates module management into first-party routes, all requiring `modules.read`:

| Route | Purpose |
| --- | --- |
| `/modules` | Legacy entry point (installed modules) |
| `/modules/installed` | Installed modules, status, start/stop/restart/remove and incomplete-install recovery |
| `/modules/catalog` | Official catalog entries and installation controls |
| `/modules/operations` | Active module jobs and the operation accepted in this UI session |
| `/modules/settings` | Docker runtime diagnostics, host capabilities, and manager-only Manifest v2 installation |

A successful module lifecycle request opens `/modules/operations` to show its progress.
When the catalog or Docker-inspection endpoint is unavailable, installed-module controls remain independent.
Catalog entries are only shown as installable after the official catalog provides an immutable, verified image digest.
The module registry is authoritative: if it cannot be read, mutations are disabled rather than risking an unsafe replacement.

The pages keep the existing `modules.read` and `modules.manage` permissions and CSRF checks.
A module's own entry point (for example `/modules/nvr.module`) remains separate from the management routes.
The Core menu highlights Modules for all four management pages, not for a module's own page.

## AI Agent discovery

The source repository `DeadSoulf/home-ai-core_ai` alone does **not** create a catalog entry or install a container.
Publish and verify an accessible immutable OCI image first, then add a valid Manifest v2 to
`DeadSoulf/home-ai-core_modules/catalog.json`. Do not substitute a tag or fictional SHA256.
The installed module can then contribute its own navigation metadata.
The protected Core→AI web gateway remains a separate integration task; a menu item by itself does not make the agent's chat reachable.
