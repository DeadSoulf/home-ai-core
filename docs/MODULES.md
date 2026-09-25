# Home AI Core — Module Manager

## Purpose

Version 0.0.6 introduces a common lifecycle manager for Home AI Core subsystems.

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
- restart one module without restarting the whole Core
- watchdog/recovery policies
- module resource limits
- signed external module packages
