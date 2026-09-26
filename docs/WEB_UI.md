# Home AI Core — Web UI Structure

## Purpose

The Web Core uses a permanent left sidebar so each subsystem has its own workspace.

The home page is intentionally kept minimal: it shows only system statistics and active errors/warnings.

## Sidebar

The sidebar is organized by user task rather than by implementation module. Empty groups are
not rendered when the current user has no permission for any item in that group.

Current structure:

```text
Обзор
└── Главная

Сервер
├── Система
├── Сеть
├── Хранилище
└── Виртуализация

Сервисы
├── Файлы
├── Камеры
├── Умный дом
└── Автоматизация

AI
├── AI
└── AI / GPU

Управление
├── Пользователи
└── Настройки
```

The route paths remain stable for backward compatibility.

## Mobile layout

The Web UI uses the same routes and permissions on desktop, tablet and phone.

At widths up to 860 px:

- the permanent desktop sidebar becomes an off-canvas navigation drawer
- navigation links and the menu button use 48 px touch targets
- tapping the backdrop, selecting a navigation link or pressing Escape closes the drawer
- the closed drawer is removed from keyboard focus and exposed with matching ARIA state
- cards, forms, storage layouts, user metadata and permission controls collapse to one column
- form controls use a 16 px font size to avoid unwanted browser zoom on focus
- inputs, selects and primary buttons use approximately 48 px touch targets
- long paths, addresses, code and status text wrap or scroll instead of widening the page
- disk-management dialogs become bottom-sheet style panels
- safe-area insets are respected on phones with notches and home indicators
- keyboard focus uses a visible accent outline

At widths up to 600 px, the top bar uses a compact grid, action rows stack vertically and
key/value details are easier to scan on narrow screens.

At widths up to 420 px, secondary Core/version metadata is hidden from the top bar so the
page title and language selector remain usable.

The viewport uses `viewport-fit=cover` to enable safe-area handling on supported phones.

Desktop behavior and route URLs remain unchanged.

## Routes

```text
/               Главная
/system         Система
/network        Сеть
/storage        Хранилище
/hypervisor     Виртуализация
/files          Файлы
/cameras        Камеры
/smart-home     Умный дом
/automation     Автоматизация
/ai             AI
/admin          AI / GPU
/users          Пользователи
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
- Linux interface inventory
- current IPv4 addresses
- MAC, MTU, link state and default-route marker
- DHCP/static IPv4 selector per interface
- static IP, netmask, gateway and DNS fields
- constrained privileged Network Helper
- persistent NetworkManager, systemd-networkd and Debian ifupdown configuration
- WireGuard profile status and connect/disconnect
- create, edit, save and remove WireGuard profiles for users with network.manage

Future content:

- advanced routes
- DNS
- diagnostics

## Storage

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

Current content:

- Camera Core status overview
- persistent RTSP camera registry
- create, edit and delete cameras
- RTSP URL, username and password camera form
- ONVIF XAddr kept as hidden internal state instead of a user-facing field
- compact ONVIF discovery list showing only camera IP/address and the Use action
- local encrypted password storage
- ONLINE/OFFLINE/disabled network state
- lightweight RTSP endpoint checks
- combined camera discovery: Hikvision SADP + ONVIF WS-Discovery + local IPv4 camera-port scan
- native Hikvision SADP discovery over UDP 37020 even when ONVIF/RTSP services are disabled
- discovery rows show useful camera metadata instead of technical URLs
- manufacturer/source, model, firmware and serial number are shown when available
- SADP device model, firmware and serial metadata reused by the camera form
- compact discovery list remains IP/address only
- automatic ONVIF Media Profile retrieval when ONVIF is available
- ONVIF device information: manufacturer, model, firmware version and serial number
- persistent camera device metadata shown on camera cards
- ONVIF PTZ controls for cameras exposing a PTZ profile
- touch-friendly press-and-hold pan/tilt/zoom controls
- on-demand Live View preview with automatically refreshed JPEG frames
- automatic RTSP URI selection with the highest-resolution profile preferred
- alternate ONVIF profiles available for manual main/substream choice
- real RTSP media probe through ffprobe
- video codec, resolution, FPS and audio codec diagnostics
- JPEG snapshots through ffmpeg
- permission-aware controls using `cameras.view` and `cameras.manage`

The camera list never returns camera passwords through the normal Web API.

Next stages:

- continuous browser video transport integrated with the recorder
- continuous/event recording
- archive and timeline
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
