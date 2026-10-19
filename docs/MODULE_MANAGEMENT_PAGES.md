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

## AI Agent upgrades

On the installed modules page an administrator may check the official catalog for
a newer immutable AI Agent image. For a running CPU-only `ai.agent` instance,
the catalog provides an explicit, confirmation-protected **Update module**
action; it creates an authenticated `module.upgrade` job. The job reports
Docker download and verification progress via the existing Operations page.

Core verifies a strictly greater module version and that both old and new images
are immutable digests. The privileged helper pulls the new image **before**
stopping the old AI container, stages the replacement using the same persistent
`/data` bind mount, waits for the candidate's Docker health check, and retains
the stopped previous container until the new manifest is recorded. If startup
or registry persistence fails, it attempts to restore the previous container.
No installed models or conversation data are intentionally deleted.

This first upgrade path is deliberately **AI Agent CPU-only**. Containers with
NVIDIA GPU assignment or custom device requests and other module IDs remain
protected against replacement. The previous container rollback does not roll
back arbitrary data-schema changes performed inside `/data`; take a data
backup before any release that changes persistent on-disk formats. If a host
crashes mid-swap, a staged backup may require operator recovery.
