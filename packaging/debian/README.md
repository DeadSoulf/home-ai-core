# Debian 13 Initial Installation

Home-AI-Core targets Debian 13 on amd64 and arm64.

## Purpose of the package

The Debian package is used for:

- first installation on a clean machine
- emergency recovery
- package-level transitions that require new system dependencies or service definitions

Routine Home-AI-Core upgrades are performed through the Web updater and do not require installing a new `.deb`.

## Installed runtime

The package installs:

```text
/usr/bin/home-ai-core
/usr/libexec/home-ai-core/home-ai-core-updater
/usr/share/home-ai-core/web/
/lib/systemd/system/home-ai-core.service
/lib/systemd/system/home-ai-core-updater.service
/usr/lib/sysusers.d/home-ai-core.conf
/etc/home-ai-core/home-ai-core.env
/usr/share/doc/home-ai-core/copyright
```

Persistent state lives under:

```text
/var/lib/home-ai-core/
```

The network-facing Core runs as the unprivileged `home-ai-core` user. The updater/storage helper runs separately as root.

## Build

```sh
sh ./scripts/build-deb.sh amd64
sh ./scripts/build-deb.sh arm64
```

Packages are created in `build/packages/`.

The GitHub Actions workflow `Build initial installer` builds downloadable amd64 and arm64 installer artifacts on demand.

## First-run access

The default listener is `127.0.0.1:8080`.

Use an SSH tunnel for remote first-run access when needed:

```sh
ssh -L 8080:127.0.0.1:8080 user@server
```

The one-time bootstrap token is stored at:

```text
/var/lib/home-ai-core/bootstrap-token
```

and is deleted after first-owner creation.

## Normal updates

After installation use **System → Updates**.

The Web updater preserves Core state/configuration, updates Core/Web/helper from a verified bundle and keeps the previous application version available for rollback.

## Removal

Package removal stops/disables the services but intentionally preserves persistent state.
