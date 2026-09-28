# Home-AI-Core v1 — Vision

Home-AI-Core is a modular server platform installed on top of a normal Debian 13 system.

It is not a replacement Linux distribution and it is not a monolithic NAS, Docker or virtualization appliance.

## Product model

```text
Debian 13
   |
   v
Home-AI-Core
   |
   +-- Core API
   +-- Authentication
   +-- Permissions
   +-- Event Bus
   +-- Job Engine
   +-- Module Manager
   +-- Update Manager
   +-- Web UI
          |
          +-- Modules
          |    +-- Containers
          |    +-- Virtualization
          |    +-- Storage
          |    +-- NAS
          |    +-- Network/VPN
          |    +-- Backup
          |    +-- AI
          |    +-- NVR
          |
          +-- Apps
```

## Goal

A user should be able to start with a minimal Debian server and add only the server capabilities they need through one coherent web interface.
