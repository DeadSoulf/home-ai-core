# Home AI Core — Roadmap

## Version plan

| Version | Milestone | Status |
|---|---|---|
| 0.0.1 | Initial Core Runtime | DONE |
| 0.0.2 | Logger, Event Bus, Config Manager | DONE |
| 0.0.3 | Integrated Web Core | DONE |
| 0.0.4 | System Monitor and live Dashboard metrics | DONE |
| 0.0.5 | Security Core: users, sessions, roles, audit | IN DEVELOPMENT |
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

## Current focus: 0.0.5

Security Core should provide:

- initial administrator creation
- password hashing
- login/logout
- session management
- roles: admin, operator, viewer
- API authorization
- failed-login throttling
- audit logging
- secure defaults for administrative functions

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
