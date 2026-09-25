# Home AI Core — Roadmap

## Version plan

| Version | Milestone | Status |
|---|---|---|
| 0.0.1 | Initial Core Runtime | DONE |
| 0.0.2 | Logger, Event Bus, Config Manager | DONE |
| 0.0.3 | Integrated Web Core | DONE |
| 0.0.4 | System Monitor and live Dashboard metrics | DONE |
| 0.0.5 | Security Core: users, sessions, roles, audit, protected Web/API | TESTING |
| 0.0.6 | Module Manager | PLANNED |
| 0.0.7 | Storage Core | PLANNED |
| 0.0.8 | Device Core | PLANNED |
| 0.0.9 | Automation Core | PLANNED |
| 0.1.0 | First AI Brain runtime | PLANNED |
| 0.2.0 | Video Surveillance / NVR Core | PLANNED |
| 0.3.0 | Hypervisor Core | PLANNED |

## Development priorities

1. Secure the Web Core before exposing administrative actions.
2. Introduce a Module Manager so major subsystems remain isolated and replaceable.
3. Build Storage and Device abstractions before higher-level automation.
4. Keep core operation local-first and independent of Internet availability.
5. Keep experimental self-development isolated behind tests, sandboxing and rollback.
6. Preserve portability between the current Proxmox VM and the future physical Debian server.

## Current focus: 0.0.5 testing

The current `develop` branch contains:

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

Before 0.0.5 moves to `main`, it must pass the automated tests and browser verification on the Debian development server.

## Next: 0.0.6 Module Manager

The Module Manager will define a common lifecycle for subsystems:

```text
discover -> initialize -> start -> health -> stop
```

Planned initial modules:

- Security
- Web
- System Monitor
- Storage
- Devices
- Automation
- AI
- Video
- Hypervisor

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
