# Debian Runtime Layout

Home-AI-Core runs as native Debian 13 services.

## Service identities

The network-facing Core runs as:

```text
user:  home-ai-core
group: home-ai-core
```

It is intentionally unprivileged.

A separate privileged helper runs as root:

```text
/usr/libexec/home-ai-core/home-ai-core-updater
```

The helper accepts only typed operations over a Unix socket and verifies the peer UID before processing requests.

## Filesystem layout

```text
/usr/bin/home-ai-core
/usr/libexec/home-ai-core/home-ai-core-updater
/usr/share/home-ai-core/web/

/etc/home-ai-core/
/var/lib/home-ai-core/
/var/lib/home-ai-core/update/
/run/home-ai-core-updater.sock
```

## Default network exposure

Core listens on:

```text
127.0.0.1:8080
```

unless the administrator explicitly changes configuration.

## Privilege boundary

The public Core retains restrictive systemd hardening and does not gain root capabilities for disk or update management.

Privileged operations are delegated to the helper. Current helper responsibilities include:

- verified Core/Web/helper bundle installation and rollback
- guarded block-device operations
- SMART/filesystem/LVM inspection that requires device access

The Web client never supplies an executable path or arbitrary shell command.

## Installation and updates

The Debian package creates the service accounts, units and initial binaries.

Normal application updates then use verified update bundles from the Web UI. The Debian package is retained for initial installation, recovery and package-level dependency/service transitions.
