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
│   ├── users
│   ├── password hashing
│   ├── sessions
│   └── login / logout
├── permissions/
│   ├── admin
│   ├── operator
│   └── viewer
├── audit/
│   └── security and administrative audit log
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
├── storage/
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
│   └── integrated HTTP server
├── api/
└── ui/
    ├── Dashboard
    ├── AI
    ├── Smart Home
    ├── Cameras
    ├── Hypervisor
    ├── Storage
    ├── Memory
    ├── Automation
    ├── System
    ├── Security
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
├── SECURITY.md       (planned)
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
