# Home AI Core — Module Manager

## Purpose

Version 0.0.6 introduced the common lifecycle manager. Version 0.0.45 added dependency-safe module restart, and 0.0.46 enables bounded runtime watchdog recovery.

Version 0.0.61 adds the Project Module Catalog foundation. Home AI Core is being separated into a minimal platform core plus project-defined modules. The catalog is intentionally read-only in this milestone: it describes module identity, dependencies, permissions, delivery state and the runtime modules that currently implement each bundled subsystem. Installation actions are enabled only after a subsystem has been extracted into a separately installable package.

Each managed subsystem follows the lifecycle:

```text
register
  ↓
initialize
  ↓
start
  ↓
health
  ↓
stop
```

## Common module interface

Every module implements:

- unique module name
- dependency list
- initialize
- start
- stop
- health
- optional health message

Lifecycle states:

```text
registered
initialized
running
stopped
failed
```

Health states:

```text
unknown
healthy
degraded
unhealthy
```

## Dependency ordering

Module Manager resolves dependencies before initialization/start.

Example:

```text
security ─────┐
update ───────┤
system-monitor├──> web
storage-monitor┘
```

If a dependency is missing or a cycle is detected, startup is refused.

Shutdown happens in reverse start order, so dependent modules stop before the services they depend on.

## Restart and watchdog recovery

A running module can be restarted without restarting the whole Core. The restart scope includes
currently running transitive dependents: dependents stop first in reverse dependency order, then
the target and its dependents start again in topological order.

The runtime watchdog automatically attempts recovery only for modules reporting `unhealthy`.
A `degraded` module is deliberately not restarted automatically because degraded state can
represent a valid environmental limitation or resource warning.

Default configuration:

```text
modules.watchdog_enabled=true
modules.watchdog_interval_seconds=10
modules.watchdog_max_restarts=3
modules.watchdog_cooldown_seconds=30
```

The retry limit is per unhealthy episode. A later healthy watchdog pass resets the episode attempt
counter, while total restart count remains tracked for diagnostics.

## Current managed modules

### security

Responsibilities:

- users
- password verification
- sessions
- roles
- audit

### update

Responsibilities:

- GitHub update check
- pull/build/test workflow
- rollback state
- restart coordination

### system-monitor

Responsibilities:

- CPU
- RAM
- root filesystem
- load
- uptime

It reports degraded health when RAM or root filesystem usage reaches the current critical threshold.

### storage-monitor

Responsibilities:

- storage inventory
- configured video/personal storage
- online/offline detection
- read-only detection
- capacity thresholds

Configured storage problems cause degraded health.

### web

Responsibilities:

- authenticated HTTP interface
- API
- sidebar UI

Dependencies:

```text
security
update
system-monitor
storage-monitor
```

## API

Authenticated users can read Module Manager state:

```text
GET /api/modules
```

Response shape:

```json
{
  "modules": [
    {
      "name": "web",
      "state": "running",
      "health": "healthy",
      "message": "HTTP interface is accepting connections.",
      "dependencies": [
        "security",
        "update",
        "system-monitor",
        "storage-monitor"
      ]
    }
  ]
}
```

## Web UI

Open:

```text
Система -> Модули ядра
```

The Web UI shows:

- lifecycle state
- health
- health details
- module dependencies

The Home page also consumes module health and adds degraded/unhealthy modules to the Errors and Warnings section.

## Future extensions

Module Manager is the base for registering:

- Storage Core
- Device Core
- Automation Core
- Camera/NVR Core
- AI Brain
- Memory Core
- Hypervisor Core
- Backup Core

Future versions can extend it with:

- optional/reloadable modules
- module configuration schema
- Web/API control for explicitly requested module restart
- configurable per-module recovery policies
- configuration reload
- module resource limits
- signed external module packages


## Notification Center

Since 0.0.47 the Event Bus supports both topic-specific handlers and global subscribers.
Events may optionally include a source and severity. Only events with an explicit user-visible
severity (`info`, `warning`, `error`, `critical`) enter the bounded in-memory
Notification Center.

0.0.48 exposes recent notifications through the permission-protected
`GET /api/notifications` endpoint and renders them on the System page. This is intentionally
an in-memory operational feed; durable retention/export remains a later feature.


## 0.0.61 Project Module Catalog

The trusted module catalog is registered by Core at startup and is exposed through:

```text
GET /api/module-catalog
```

It requires `system.view`. The dedicated Web page is:

```text
/modules
```

Catalog entries distinguish:

- `core` — mandatory Home AI Core platform components
- `running` — bundled subsystem with all declared runtime modules running
- `installed` — bundled subsystem present but not represented by a running lifecycle module
- `available` — package is installable from the trusted project catalog
- `planned` — project module is defined but has not yet been extracted into an installable package

Each manifest carries an id, display name, version, description, project dependencies, permission names and the current runtime-module mapping.

The current 0.0.61 catalog is a migration layer. It does not claim that planned modules can already be installed, and it does not execute arbitrary URLs or shell commands.

### Target architecture

```text
Home AI Core
  ├─ Web UI / Security / Update / Event Bus / Module Manager
  └─ Project Module Catalog
       ├─ Storage Core
       ├─ Network Core
       ├─ Files Core
       ├─ Hypervisor Core
       ├─ Camera Core
       ├─ NVR Core
       ├─ Cluster Core
       ├─ AI Core
       ├─ Smart Home Core
       └─ Automation Core
```

The extraction sequence keeps the current working implementation operational while one subsystem at a time moves from `bundled=true` to a separately installable package. Large modules should ultimately run outside the Core process so a module crash cannot terminate the Core Web/Security control plane.


## 0.0.62 Module Installer

Version 0.0.62 adds the first safe install/uninstall path for trusted project modules.

The installer intentionally has no arbitrary package URL, command, script, or shell execution surface.
It only changes a local installation-state registry:

```text
runtime/modules/installed.tsv
```

The registry is written atomically with owner-only permissions. Unknown/stale entries are ignored,
while malformed state values fail closed.

Installer rules:

- Core components cannot be removed.
- Only manifests explicitly marked `installable=true` can change installation state.
- Installation is refused when a declared dependency is not installed.
- Uninstallation is refused while another installed module depends on the target.
- Runtime-backed modules report a restart-required transition.
- The authenticated mutation API remains behind the normal `system.manage`, request-header and
  session-bound CSRF guards.

API:

```text
POST /api/module-catalog/install
POST /api/module-catalog/uninstall
```

Both accept URL-encoded `id=<module-id>`.

### First migration module: Cluster Core

Cluster Core is the first bundled subsystem controlled by Module Installer. It remains installed by
default for upgrade compatibility, but an administrator can remove it from the Modules page. After
restart, its lifecycle module is not registered and Cluster Web access redirects back to the module
catalog. It can then be installed again from the same catalog and restored on the next restart.

This milestone deliberately keeps the Cluster implementation compiled into the Core binary. The next
extraction stage can move its implementation into a separately delivered module package without
changing the catalog/install-state contract introduced here.
