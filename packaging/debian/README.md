# Debian Packaging Foundation

Home-AI-Core targets Debian 13 as its first supported host platform.

## Files

- `home-ai-core.service` — systemd unit for the unprivileged network-facing Core.
- `home-ai-core.sysusers` — system user declaration.
- `home-ai-core.env.example` — optional bootstrap environment overrides.

## Intended installed layout

```text
/usr/bin/home-ai-core
/etc/home-ai-core/
/var/lib/home-ai-core/
/run/home-ai-core/
```

The `home-ai-core` process runs as the dedicated `home-ai-core` account.

The future privileged helper described by ADR-0003 will use a separate systemd unit and a separate security profile. It must not be added to this unit by granting root privileges to the public Core.

## Packaging status

This directory is not yet a complete Debian package definition. The current phase establishes the runtime contract and filesystem ownership model before creating `debian/control`, maintainer scripts and signed package repositories.
