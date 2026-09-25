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
| 0.0.7 | WireGuard Web editor + centralized versioning | IN DEVELOPMENT |
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

## Current focus: 0.0.7

Current development work:

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

0.0.7 requires a development-server pull, clean build and the expanded test suite before
it can be promoted as a stable version.

Patch versions after 0.0.7 are intentionally not pre-assigned to individual roadmap items.
Each Core/runtime change advances to the next version, while larger minor milestones such as
0.1.0 remain roadmap targets.

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
