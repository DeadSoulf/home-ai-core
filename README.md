# Home AI Core

Home AI Core is a local-first autonomous server platform designed for:

- artificial intelligence
- smart home control
- personal data management
- server management
- hypervisor management
- video surveillance and recording
- automation
- long-term memory
- controlled self-development

## Current stable version

0.0.5

## Current development version

0.0.7 — WireGuard Web editor + centralized automatic version discipline

The repository root `VERSION` file is the single source of the compiled Core version.

## Platform

- Debian 13
- Linux x86-64
- C++20

## Main modules

- Core
- AI
- Memory
- Devices
- Automation
- Server
- Data
- Security
- Hypervisor
- Video
- Web/API

## Documentation

- [Project Map](docs/PROJECT_MAP.md)
- [Roadmap](docs/ROADMAP.md)
- [Development Workflow](docs/WORKFLOW.md)
- [Security](docs/SECURITY.md)
- [Storage Monitoring](docs/STORAGE.md)
- [Web UI](docs/WEB_UI.md)
- [Server Updates](docs/UPDATES.md)
- [Module Manager](docs/MODULES.md)

## Branches

- `main` — stable, tested code
- `develop` — active development and integration

The development server should normally track `develop`.

## Status

The stable 0.0.5 foundation includes Core Runtime, logging, Event Bus, configuration, integrated Web Core, system monitoring, Security Core, authenticated Web/API access, and storage monitoring for mounted video/personal-data disks.
# Service, languages and GPU administration

- [Запуск без SSH: systemd-служба / Service installation](docs/SERVICE.md)
- [Администрирование GPU, русский и English / GPU administration and languages](docs/ADMINISTRATION.md)


## Development Security Core

The current `develop` branch uses a central SQLite identity/security database with roles, granular per-user permissions, persistent sessions, session revocation, audit history and a complete Users administration page. Existing legacy users are migrated without changing their password hashes.
