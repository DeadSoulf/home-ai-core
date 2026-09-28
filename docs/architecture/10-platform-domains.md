# Platform Domains

Home-AI-Core separates platform control from workload implementation.

## Control Plane

The Control Plane is provided by the Core and is responsible for:

- identity
- authentication
- authorization
- node registry
- module registry
- configuration
- event bus
- jobs
- audit
- updates
- health and capability discovery

## Data Plane

The Data Plane is implemented by modules and services.

Examples:

- file storage
- object/media storage
- SMB/NFS
- camera ingest and recording
- Docker workloads
- virtual machines
- AI inference
- smart-home integrations
- backup engines

## User-facing domains

### Files
Personal documents and general file storage.

### Photos and Video
Photo/video ingest, organization, metadata, thumbnails and future mobile synchronization.

### Surveillance
Cameras, streams, recordings, retention and AI events.

### Home Automation
Devices, states, automations and external smart-home systems.

### Applications
Self-hosted services such as media servers, collaboration tools and automation platforms.

### AI
Models, runtimes, inference endpoints, agents and approved platform tools.

### Infrastructure
Storage, networking, compute, containers, virtualization, backups and hardware.

## Architectural rule

User-facing domains may span multiple modules, but no domain is allowed to bypass Core identity, permissions, jobs, events or audit controls when performing privileged platform operations.
