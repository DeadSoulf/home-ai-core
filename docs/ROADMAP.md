# Home AI Core — Roadmap

## Version plan

| Version | Milestone | Status |
|---|---|---|
| 0.0.1 | Initial Core Runtime | DONE |
| 0.0.2 | Logger, Event Bus, Config Manager | DONE |
| 0.0.3 | Integrated Web Core | DONE |
| 0.0.4 | System Monitor and live Dashboard metrics | DONE |
| 0.0.5 | Security Core + protected Web/API + storage monitoring foundation | DONE |
| 0.0.6 | Module Manager + hot-plug storage integration | IN DEVELOPMENT |
| 0.0.7 | Full Storage Core: health, pools, quotas, retention | PLANNED |
| 0.0.8 | Device Core | PLANNED |
| 0.0.9 | Automation Core | PLANNED |
| 0.1.0 | First AI Brain runtime | PLANNED |
| 0.2.0 | Video Surveillance / NVR Core | PLANNED |
| 0.3.0 | Hypervisor Core | PLANNED |

## Development priorities

1. Introduce a Module Manager so major subsystems remain isolated and replaceable.
2. Continue the Storage Core safely before allowing destructive disk operations.
3. Build Device abstractions before higher-level automation.
4. Keep core operation local-first and independent of Internet availability.
5. Keep experimental self-development isolated behind tests, sandboxing and rollback.
6. Preserve portability between the current Proxmox VM and the future physical Debian server.

## Stable: 0.0.5

Validated on the Debian development server:

- local user storage
- PBKDF2-HMAC-SHA256 password hashing
- first-run administrator creation
- login/logout
- 8-hour local sessions
- roles: admin, operator, viewer
- protected Web Core
- protected API routes
- admin-only configuration writes
- failed-login throttling
- audit logging
- security response headers
- storage monitoring foundation for mounted video/personal-data disks
- ONLINE/OFFLINE, filesystem, capacity and read-only reporting
- authenticated `/api/storage` endpoint and live storage dashboard

## Current focus: 0.0.6

Completed in the current development branch:

- sysfs block-device discovery
- manual "Проверить новые диски" control
- automatic hot-plug polling in the Web UI
- notification when a new unused disk appears
- disk-management menu in the Web UI
- role assignment for video / personal files
- privileged mount and unmount through a validated helper
- guarded EXT4 format and signature removal operations
- filtering for mounted, swap and Linux-holder-backed devices
- `GET /api/storage/devices`
- `POST /api/storage/action`
- persistent sidebar navigation with subsystem sections
- main page reduced to system statistics and active errors
- existing disk/network/settings controls moved into their own sections

Still planned for 0.0.6:

- common module interface
- module registration
- lifecycle state tracking
- dependency ordering
- health reporting
- clean shutdown ordering
- module status exposure in the Web UI

The Module Manager lifecycle will be:

```text
discover -> initialize -> start -> health -> stop
```

## Later major capabilities

### Smart home

- unified device model
- MQTT, Zigbee, Modbus, GPIO, HTTP, TCP/UDP, BLE
- rules, triggers and scenarios
- learned automation proposals

### Personal data

- indexed file storage
- metadata and content search
- versions and backups
- controlled AI access

### Video surveillance

- RTSP / ONVIF
- continuous and event recording
- pre-event buffer
- archive and playback
- motion/object analytics

### Hypervisor

- KVM-based VM management
- disks and virtual networks
- snapshots and backups
- sandbox VMs for testing generated code

### AI

- local inference runtime
- long-term memory
- planning
- experience feedback
- controlled self-development
