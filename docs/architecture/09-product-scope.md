# Product Scope

## Mission

Home-AI-Core is a self-hosted home infrastructure platform that turns one or more Debian servers into a unified private system for personal data, home automation, video surveillance, applications and local AI.

The system is designed for private ownership of data and local operation first.

## Primary workloads

### Personal Cloud

- personal documents and files
- family photos and videos
- automatic upload from future mobile clients
- shared family storage
- backup and restore
- media indexing and search

### Video Surveillance

- IP camera integration
- live viewing
- recording
- retention policies
- event detection
- optional AI analysis of video streams

### Smart Home

- Home Assistant or equivalent integrations
- sensors, switches, climate, lighting and security devices
- automation workflows
- event exchange with the Home-AI-Core event bus

### AI Core

- local LLM and multimodal runtimes
- GPU/accelerator discovery and scheduling
- model management
- access to approved Home-AI-Core capabilities
- system diagnostics and assisted configuration
- smart-home assistance and automation
- search and reasoning over user-owned data where explicitly enabled

AI is not granted unrestricted root access. All actions pass through the same permission, policy and audit boundaries as other actors.

### Multi-server Operation

A household may contain multiple Home-AI-Core nodes.

Nodes can contribute:

- CPU
- RAM
- GPU/accelerators
- storage
- application capacity
- video-processing capacity

The platform should present those nodes as one managed home infrastructure environment while preserving node-level failure isolation.

### Mobile Clients

Future mobile applications should support:

- automatic photo/video upload
- document upload
- background synchronization
- resumable transfers
- remote access through secure identity and networking
- selective download/offline access
- notifications and system status

## Non-goals for the initial Core

The Core itself does not implement NAS protocols, camera processing, smart-home logic, model inference or mobile synchronization.

It provides the contracts and control plane that allow those capabilities to exist as modules and applications.
