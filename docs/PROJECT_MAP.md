# Home AI Core — Project Map

This document is the navigation map for the Home AI Core repository.

## Top-level structure

```text
home-ai-core/
├── main.cpp
├── CMakeLists.txt
├── config/
├── core/
├── security/
├── ai/
├── memory/
├── devices/
├── automation/
├── server/
├── hypervisor/
├── video/
├── data/
├── web/
├── tests/
└── docs/
```

## Core

```text
core/
├── runtime/
│   └── CoreRuntime
│       Starts and stops the core and coordinates core services.
├── modules/
│   ├── Module
│   │   Common module lifecycle and health interface.
│   └── ModuleManager
│       Dependency ordering, lifecycle state, health and reverse shutdown.
├── events/
│   └── EventBus
│       Internal event exchange between modules.
├── logging/
│   └── Logger
│       Central logging.
└── config/
    └── ConfigManager
        Loads and saves core configuration.
```

## Security

```text
security/
├── auth/
│   ├── SecurityManager
│   │   authentication, authorization and security operations
│   └── UserDatabase
│       central SQLite users, permissions, sessions and audit store
├── permissions/
│   ├── role defaults: admin / operator / viewer
│   └── per-user inherit / allow / deny overrides
├── audit/
│   └── central database-backed security and administrative audit
└── sandbox/
    └── isolation for dangerous or experimental operations
```

## AI

```text
ai/
├── brain/
│   └── primary AI runtime
├── planner/
│   └── task and action planning
├── learning/
│   └── learning from events and outcomes
└── self-development/
    └── controlled generation and testing of new skills/modules
```

## Memory

```text
memory/
├── semantic/
│   └── facts and knowledge
├── episodic/
│   └── event history
└── experience/
    └── results of AI actions and feedback
```

## Devices

```text
devices/
├── manager/
│   └── unified device model and lifecycle
└── drivers/
    ├── MQTT
    ├── Zigbee
    ├── Modbus
    ├── GPIO
    ├── HTTP
    ├── TCP / UDP
    ├── BLE
    └── ONVIF
```

## Automation

```text
automation/
├── rules/
├── triggers/
└── scenarios/
```

## Server management

```text
server/
├── system/
│   └── SystemMonitor
├── network/
│   ├── NetworkInterfaceManager
│   │   interface inventory and unprivileged DHCP action client
│   ├── helper/NetworkHelperMain
│   │   privileged validated DHCP helper
│   ├── WireGuardManager
│   └── VpnService
├── storage/
│   ├── StorageMonitor
│   │   mounted storage, hot-plug discovery, capacity, roles and online/offline state
│   ├── StoragePool
│   │   multi-disk placement policies, reserves and write-target selection
│   ├── DiskOperations
│   │   unprivileged client for guarded disk actions and UUID remounts
│   └── StorageHelperMain
│       privileged validated mount/unmount/format helper
├── processes/
└── backup/
```

## Hypervisor

```text
hypervisor/
├── kvm/
├── vm/
├── storage/
├── network/
├── snapshots/
└── console/
```

The planned hypervisor layer will use Linux KVM as the virtualization foundation while Home AI Core provides its own management layer.

## Video surveillance

```text
video/
├── cameras/
├── rtsp/
├── onvif/
├── recorder/
├── archive/
├── playback/
├── motion/
└── analytics/
```

## Personal data

```text
data/
├── files/
├── index/
├── search/
└── versions/
```

## Web Core

```text
web/
├── server/
│   └── integrated HTTP server and API routing
├── api/
└── ui/
    ├── WebUi
    │   shared sidebar shell and section rendering
    ├── Home
    │   statistics and active errors only
    ├── Network
    ├── Storage
    ├── Cameras
    ├── Smart Home
    ├── Automation
    ├── AI
    ├── Users
    ├── Hypervisor
    ├── System
    └── Settings
```

## Configuration

```text
config/
└── home-ai.conf
```

## Tests

```text
tests/
├── core tests
├── security tests
├── web tests
├── device tests
└── integration tests
```

## Documentation

```text
docs/
├── PROJECT_MAP.md
├── ROADMAP.md
├── WORKFLOW.md
├── ARCHITECTURE.md   (planned)
├── SECURITY.md
├── STORAGE.md
├── WEB_UI.md
├── UPDATES.md
├── MODULES.md
├── NETWORK.md
├── API.md            (planned)
└── BUILD.md          (planned)
```

## Runtime data layout

Runtime and private data must not be committed to Git.

```text
/etc/home-ai/
    configuration

/var/lib/home-ai/
    database/
    memory/
    models/
    devices/
    video/
    backups/

/var/log/home-ai/
    core and audit logs

/opt/home-ai/
    installed binaries/modules
```
