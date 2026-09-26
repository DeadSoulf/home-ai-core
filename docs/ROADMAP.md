# Home AI Core — Roadmap

## Version plan

| Version | Milestone | Status |
|---|---|---|
| 0.0.1 | Initial Core Runtime | DONE |
| 0.0.2 | Logger, Event Bus, Config Manager | DONE |
| 0.0.3 | Integrated Web Core | DONE |
| 0.0.4 | System Monitor and live Dashboard metrics | DONE |
| 0.0.5 | Security Core + protected Web/API + storage monitoring foundation | DONE |
| 0.0.6 | Module Manager + hot-plug storage integration + Storage Pool foundation | DONE |
| 0.0.7 | WireGuard Web editor + centralized versioning | DEVELOPMENT SNAPSHOT |
| 0.0.8 | Logical permission-aware Web sidebar and navigation | DEVELOPMENT SNAPSHOT |
| 0.0.9 | Responsive mobile Web UI and off-canvas navigation | DEVELOPMENT SNAPSHOT |
| 0.0.10 | Refined phone/tablet Web UI and touch ergonomics | DEVELOPMENT SNAPSHOT |
| 0.0.11 | Camera Core foundation: secure RTSP registry and health monitoring | DEVELOPMENT SNAPSHOT |
| 0.0.12 | ONVIF discovery + real RTSP media probe + snapshots | DEVELOPMENT SNAPSHOT |
| 0.0.13 | Fix camera inventory projection after ONVIF migration | DEVELOPMENT SNAPSHOT |
| 0.0.14 | Automatic RTSP discovery from ONVIF Media Profiles | DEVELOPMENT SNAPSHOT |
| 0.0.15 | ONVIF manufacturer/model/firmware/serial metadata | DEVELOPMENT SNAPSHOT |
| 0.0.16 | ONVIF PTZ controls + browser Live View preview | DEVELOPMENT SNAPSHOT |
| 0.0.17 | Simplified ONVIF discovery list and hidden technical endpoints | IN DEVELOPMENT |
| next 0.0.x | Recorder + continuous video transport + archive/event pipeline | PLANNED |
| next 0.0.x | Video analytics | PLANNED |
| next 0.0.x | Full Storage Core: health, quotas, retention and recorder/file integration | PLANNED |
| next 0.0.x | Device Core | PLANNED |
| next 0.0.x | Automation Core | PLANNED |
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

## Completed development milestone: 0.0.6

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
- Web-based GitHub update detection
- fast-forward repository update with automatic build and tests
- rollback when build/tests fail
- restart into the updated binary after confirmation
- central SQLite user/security database shared by server modules
- one-time migration of legacy users and audit without password loss
- persistent sessions and session revocation
- role defaults plus per-user allow/deny permission overrides
- backend permission enforcement by subsystem
- complete Users page: CRUD, roles, passwords, permissions, sessions and audit
- live Web update progress with real Ninja/CTest counters and expandable logs
- filesystem UUID discovery and UUID-based mount paths for newly managed disks
- multi-disk video and home-files pools
- one physical disk can belong to both storage roles
- most-free, sequential, balanced and pinned pool placement policies
- per-volume reserve thresholds by percentage and GB
- automatic best-effort remount of UUID-managed storage after server restart
- network interface inventory in Web UI
- DHCP address request/renew through a constrained privileged Network Helper
- NetworkManager, systemd-networkd, dhclient and udhcpc DHCP backend support
- DHCP / static IPv4 mode per interface with netmask, gateway and DNS
- persistent IPv4 configuration for NetworkManager, systemd-networkd and Debian ifupdown
- rollback backups for managed network configuration

Module Manager implemented in the current development branch:

- common module interface
- module registration
- lifecycle state tracking
- dependency ordering
- health reporting
- clean reverse shutdown ordering
- authenticated `GET /api/modules`
- module status exposure in the System page
- degraded/unhealthy module reporting on the Home page

0.0.6 passed the development-server build and 14-test suite before the 0.0.7 WireGuard/versioning work began.

The Module Manager lifecycle will be:

```text
discover -> initialize -> start -> health -> stop
```

## Development snapshot: 0.0.7

Implemented in the 0.0.7 snapshot:

- editable WireGuard profiles in the Network Web UI
- create, edit and remove WireGuard `*.conf` profiles
- profile contents available only to users with `network.manage`
- atomic WireGuard configuration saves with file mode `0600`
- WireGuard private keys excluded from the normal profile inventory and audit details
- active tunnels are not silently restarted when a profile is saved
- repository-root `VERSION` as the single compiled version source
- `scripts/bump-version.sh` for the next patch/minor/major version
- GitHub version guard for Core/runtime code changes
- VPN profile save/load/remove coverage in the test suite

## Development snapshot: 0.0.8

Implemented in the 0.0.8 snapshot:

- task-oriented sidebar groups: Overview, Server, Services, AI and Management
- Home isolated as the single Overview entry
- System, Network, Storage and Virtualization grouped under Server
- Files, Cameras, Smart Home and Automation grouped under Services
- AI runtime and GPU assignment grouped together
- Users and general Settings grouped under Management
- empty navigation groups hidden automatically according to user permissions
- storage navigation label clarified from Disks to Storage
- ambiguous Administration navigation entry renamed to AI / GPU
- existing route URLs preserved for backward compatibility
- Web UI tests cover group ordering and permission-aware visibility

## Development snapshot: 0.0.9

Implemented in the 0.0.9 snapshot:

- responsive off-canvas navigation below 860 px
- sticky mobile top bar
- single-column cards and forms on narrow screens
- mobile disk-management bottom sheet
- 16 px mobile form controls
- accessibility state for hidden navigation

## Development snapshot: 0.0.10

Implemented in the 0.0.10 snapshot:

- viewport-fit=cover safe-area support
- 48 px touch targets for navigation, inputs, selects and buttons
- compact grid-based phone header
- extra-compact treatment below 420 px
- improved key/value readability on narrow screens
- visible keyboard focus states
- better wrapping and horizontal overflow handling for long content
- mobile cards protected against width overflow
- improved touch behavior and reduced accidental zoom/highlight effects
- expanded Web UI tests for mobile layout guarantees

## Development snapshot: 0.0.11

Implemented in the 0.0.11 snapshot:

- secure persistent RTSP camera registry
- encrypted camera passwords
- camera CRUD and permissions
- lightweight endpoint health checks
- Web camera management

## Development snapshot: 0.0.12

Implemented in the 0.0.12 snapshot:

- persistent ONVIF XAddr with automatic database migration
- WS-Discovery for ONVIF NetworkVideoTransmitter devices
- ONVIF discovery results in Web UI
- real RTSP media diagnostics through ffprobe
- codec, resolution, FPS and audio codec reporting
- JPEG snapshots through ffmpeg
- parser tests for ONVIF XML and ffprobe output

## Development snapshot: 0.0.13

Implemented in the 0.0.13 snapshot:

- corrected the camera inventory SQL projection after adding ONVIF XAddr
- restored CameraManager list/test consistency after the schema migration

## Development snapshot: 0.0.14

Implemented in the 0.0.14 snapshot:

- ONVIF GetCapabilities support
- ONVIF GetProfiles parsing
- ONVIF GetStreamUri retrieval
- WS-Security UsernameToken PasswordDigest
- automatic highest-resolution RTSP stream selection
- alternate ONVIF profile selection
- automatic RTSP URL fill in Web UI
- sanitized RTSP URIs without embedded credentials

## Development snapshot: 0.0.15

Implemented in the 0.0.15 snapshot:

- ONVIF GetDeviceInformation during automatic camera setup
- manufacturer persistence and display
- model persistence and display
- firmware version persistence and display
- serial number persistence and display
- hardware ID persistence for diagnostics
- camera metadata stored separately from encrypted credentials

## Development snapshot: 0.0.16

Implemented in the 0.0.16 snapshot:

- ONVIF PTZ service discovery
- PTZ-capable Media Profile detection
- persistent PTZ XAddr and profile token
- authenticated ContinuousMove / Stop commands
- touch-friendly pan / tilt / zoom controls
- on-demand browser Live View preview
- parallel Web client workers for camera operations

## Current focus: 0.0.17

Current Camera Core/Web UI work:

- compact WS-Discovery camera list
- only the discovered camera IP/address is shown to the user
- ONVIF XAddr is retained internally and no longer displayed in the normal camera form
- discovery scopes, hardware URLs and profile URLs are hidden from the user-facing list
- existing automatic ONVIF selection and stream detection behavior is preserved
- camera discovery layout remains responsive on desktop and mobile
- Web UI tests enforce the simplified discovery presentation

0.0.17 is a presentation cleanup only; Camera Core continues to use the full ONVIF discovery
metadata internally.

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
