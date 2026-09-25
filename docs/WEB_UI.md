# Home AI Core — Web UI Structure

## Purpose

The Web Core uses a permanent left sidebar so each subsystem has its own workspace.

The home page is intentionally kept minimal: it shows only system statistics and active errors/warnings.

## Sidebar

Current sections:

```text
Главная
Система
Сеть
Диски
Камеры
Умный дом
Автоматизация
AI
Пользователи
Виртуализация
Настройки
```

## Routes

```text
/               Главная
/system         Система
/network        Сеть
/storage        Диски
/cameras        Камеры
/smart-home     Умный дом
/automation     Автоматизация
/ai             AI
/users          Пользователи
/hypervisor     Виртуализация
/settings       Настройки
```

## Main page

The main page contains only:

- Core/Web/Security status
- version
- CPU
- RAM
- system-disk utilization
- uptime
- load average
- active errors and warnings

Current error checks include:

- unavailable system-monitor API
- RAM >= 95%
- system disk >= 95%
- configured storage OFFLINE
- configured storage >= 95% full
- configured storage unexpectedly read-only

## Network

Current content:

- Web bind address
- Web port
- Web Core state
- admin form for bind/port configuration

Future content:

- interfaces
- static/DHCP addresses
- routes
- DNS
- diagnostics

## Disks

Current storage UI includes:

- mounted storage status with filesystem UUID and capacity
- hot-plug discovery and new-disk scan
- separate Video and Home Files pool summaries
- aggregate pool capacity and current next-write target
- one or both roles selectable per physical disk
- Disk Management menu
- mount/unmount
- EXT4 formatting
- signature removal
- storage placement policies: most-free, sequential, balanced and pinned
- configurable percentage/GB reserves
- pinned-disk selectors

## Cameras

Reserved for:

- RTSP/ONVIF cameras
- live view
- recording
- archive
- analytics

## Smart Home

Reserved for:

- devices
- rooms
- MQTT/Zigbee/etc.
- state
- control

## Automation

Reserved for:

- rules
- triggers
- scenarios
- execution history

## AI

Reserved for:

- AI Brain
- memory
- planner
- skills
- later self-development controls

## Users

Current content:

- current authenticated user
- role
- session state

Planned:

- user list
- create/delete users
- password reset/change
- role/permission management
- session management
- security audit view

## Virtualization

Reserved for KVM:

- virtual machines
- VM disks
- VM networks
- snapshots

## Settings

Contains general Core settings only.

Subsystem-specific settings should live inside their own sections rather than accumulating on the home page.


## System update UI

The System page shows server update state as a live percentage progress view with explicit
GitHub, CMake, build, test, activation and restart stages. Ninja and CTest counters are shown
when available, and the detailed command log can be expanded on demand.
