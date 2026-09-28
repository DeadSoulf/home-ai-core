# Physical Server Installation Acceptance

Phase 8.5 is ready for the first physical Debian 13 installation when these conditions are met.

## Package

- [x] amd64 Debian package can be built
- [x] arm64 Debian package can be cross-built
- [x] package contains Core binary
- [x] package contains production Web UI
- [x] package contains systemd service
- [x] package contains system user declaration
- [x] package contains bootstrap configuration
- [x] configuration is marked as a conffile

## Installation safety

- [x] installer requires root
- [x] installer validates Debian 13
- [x] installer validates supported architecture
- [x] service runs as `home-ai-core`, not root
- [x] default listener remains loopback-only
- [x] installer does not configure firewall/NAT/port forwarding
- [x] first-owner setup remains localhost-only
- [x] persistent state is preserved on package removal
- [x] device inventory remains visible while direct device access is denied

## Runtime validation

- [x] health endpoint can be checked after installation
- [x] Web UI root can be checked after installation
- [x] setup-status endpoint can be checked
- [x] SQLite database existence can be checked
- [x] node identity existence can be checked
- [x] systemd enabled/active state can be checked
- [x] dedicated server smoke script exists

## Deferred

- package signatures
- signed APT repository
- automatic update channels
- automatic rollback
- backup-before-upgrade orchestration
- public HTTPS gateway
