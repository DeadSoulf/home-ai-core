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

## One-command installation

On a clean Debian 13 server:

```sh
curl -fsSL https://raw.githubusercontent.com/DeadSoulf/home-ai-core/main/scripts/install.sh | sudo sh
```

The installer supports `amd64` and `arm64`, installs the required build toolchain, builds the Debian package, installs it, enables the services and verifies the Core health endpoint.

## Build

```sh
sh ./scripts/build-deb.sh amd64
sh ./scripts/build-deb.sh arm64
```

Packages are created in `build/packages/`.

The GitHub Actions workflow `Build initial installer` builds downloadable amd64 and arm64 installer artifacts on demand.

## First-run access

The default listener is `0.0.0.0:8080`.

Open the Web UI from another computer on the same private local network:

```text
http://SERVER-IP:8080/
```

Create the first owner account in the browser. No bootstrap token is generated or required. Once the first owner exists, first-run initialization is disabled.

## Normal updates

After installation use **System → Updates**.

The Web updater preserves Core state/configuration, updates Core/Web/helper from a verified bundle and keeps the previous application version available for rollback.

## Removal

Package removal stops/disables the services but intentionally preserves persistent state.
