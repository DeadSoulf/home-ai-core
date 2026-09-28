# Debian 13 Installation

Home-AI-Core targets Debian 13 on amd64 and arm64.

## Package contents

The generated Debian package installs:

```text
/usr/bin/home-ai-core
/usr/share/home-ai-core/web/
/lib/systemd/system/home-ai-core.service
/usr/lib/sysusers.d/home-ai-core.conf
/etc/home-ai-core/home-ai-core.env
```

Runtime state is created under:

```text
/var/lib/home-ai-core/
/run/home-ai-core/
```

The network-facing Core runs as the dedicated unprivileged `home-ai-core` user.

## Build a local package

On a build machine with Go 1.27+, Node.js 24+, npm and `dpkg-deb`:

```sh
sh ./scripts/build-deb.sh amd64
```

or:

```sh
sh ./scripts/build-deb.sh arm64
```

Packages are created in `build/packages/`.

## Install on a clean Debian 13 server

Copy the matching `.deb` plus `packaging/debian/install-local.sh` to the server, then run:

```sh
sudo sh ./install-local.sh ./home-ai-core_*.deb
```

The helper validates Debian 13 and amd64/arm64 before installation.

## First-run access

Core remains bound to `127.0.0.1:8080` until secure remote access is implemented.

Use an SSH tunnel from the administrator workstation:

```sh
ssh -L 8080:127.0.0.1:8080 user@server
```

Then open `http://127.0.0.1:8080/`.

The one-time bootstrap token is stored at:

```text
/var/lib/home-ai-core/bootstrap-token
```

It is deleted after first-owner creation.

## Upgrade behavior

Installing a newer `.deb`:

- preserves `/etc/home-ai-core/home-ai-core.env`
- preserves `/var/lib/home-ai-core`
- replaces the binary and Web UI
- restarts the systemd service
- lets Core apply forward SQLite migrations at startup

Automatic backup-before-upgrade and rollback remain later Update Manager work.

## Remove behavior

Package removal stops/disables the service but intentionally preserves configuration and state. Persistent state is never silently deleted.


## Hardware identification database

The Debian package depends on `pci.ids` so Home-AI-Core can resolve PCI vendor/device identifiers into human-readable hardware model names. Hardware inventory still falls back to numeric IDs if a database entry is unavailable.
