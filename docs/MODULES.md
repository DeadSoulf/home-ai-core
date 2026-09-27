# Home AI Core — Module Manager

## Purpose

Version 0.0.6 introduced the common lifecycle manager. Version 0.0.45 added dependency-safe module restart, and 0.0.46 enables bounded runtime watchdog recovery.

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
