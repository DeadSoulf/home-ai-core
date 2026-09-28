# Component Model

## Core

The Core is the smallest trusted Home-AI-Core layer. Its responsibilities are limited to:

- API routing and version negotiation
- authentication and sessions
- authorization and permission checks
- configuration registry
- module registry and lifecycle orchestration
- event bus
- job engine
- update orchestration
- audit logging
- basic host discovery required by the platform

The Core must not contain Docker-, KVM-, NAS-, AI- or NVR-specific business logic.

## Module

A Module adds host-level capability.

Examples:

- Containers
- Virtualization
- Storage
- NAS
- Backup
- Network/VPN
- AI runtime
- NVR runtime

A module declares:

- identity and version
- Core API compatibility
- dependencies and conflicts
- required OS packages
- required permissions and resources
- API routes
- events produced and consumed
- UI contributions
- lifecycle operations

## App

An App is a user-facing service running on top of one or more Modules.

Apps should normally run in an isolated runtime such as containers and must not receive unrestricted host access.

Examples include Jellyfin, Immich, Home Assistant, Nextcloud and n8n.

## Integration

An Integration connects Home-AI-Core or an installed App to another service without becoming a general host capability.

Examples include MQTT, notification providers, cloud DNS and monitoring exporters.

## Dependency rules

- Core does not depend on optional Modules.
- Apps may depend on Modules.
- Integrations may depend on Core capabilities, Modules or Apps.
- Optional components must not create circular dependencies.
- Removal must be blocked while dependants still require the component.
