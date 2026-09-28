# Debian Runtime Layout

Core v0.1 is designed as a normal system service installed on Debian 13.

## Service identity

The network-facing Core runs as:

```text
user:  home-ai-core
group: home-ai-core
```

It must never require UID 0 for normal operation.

## Filesystem layout

```text
/usr/bin/home-ai-core              executable

/etc/home-ai-core/                 bootstrap configuration
/var/lib/home-ai-core/             persistent Core state
/run/home-ai-core/                 runtime IPC/state
```

systemd creates and owns the configuration, state and runtime directories using its directory-management directives.

## Default network exposure

The initial Core listens only on:

```text
127.0.0.1:8080
```

until authentication and secure remote-access layers are implemented.

A reverse proxy or future gateway may expose authenticated APIs later.

## Hardening

The systemd unit applies a baseline including:

- no privilege escalation
- empty Linux capability sets
- strict system filesystem protection
- home-directory protection
- private temporary directory
- private device view
- kernel/control-group protection
- SUID/SGID restrictions
- realtime restrictions
- executable-memory restrictions
- limited address families

Hardening is treated as a tested runtime contract. If a future Core capability conflicts with a hardening control, the preferred solution is to move that capability into an appropriate module or the privileged helper rather than weakening the public Core globally.

## Privileged helper

The future `home-ai-privd` process is intentionally absent from Core v0.1.

Adding it requires its own executable, systemd unit, Unix socket permissions, operation protocol and security tests in accordance with ADR-0003.
