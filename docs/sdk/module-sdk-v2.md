# Module SDK v2

Module SDK v2 is the contract between Home-AI-Core and independently distributed Docker modules.

## Manifest

Canonical schema:

```text
schemas/module-manifest-v2.schema.json
```

A v2 module is a container workload. Core does not execute manifest-provided shell commands and does not install module-specific host packages.

Minimal example:

```json
{
  "schema_version": 2,
  "id": "example.module",
  "name": "Example Module",
  "description": "Independent Home-AI workload",
  "version": "0.1.0",
  "core": ">=0.1.0 <1.0.0",
  "runtime": {
    "type": "docker",
    "docker": {
      "image": "ghcr.io/example/home-ai-module@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
    },
    "health": {
      "port": 8080,
      "path": "/health"
    }
  },
  "capabilities": {
    "requires": ["host.docker"],
    "provides": ["example.service"]
  },
  "host": {
    "architectures": ["amd64", "arm64"]
  },
  "api": {
    "namespace": "example.module"
  },
  "lifecycle": ["install", "upgrade", "remove", "start", "stop", "restart"]
}
```

## Image policy

The Docker image must be pinned to an immutable `@sha256:<digest>` reference.

Tags such as `:latest` or `:1.0` are rejected by Core and by the privileged helper. This prevents a module manifest from silently resolving to different image content later.

## Runtime isolation

For the first v2 runtime profile, Core creates containers with a fixed policy:

- deterministic name `home-ai-module-<module-id>`;
- network `home-ai-modules`;
- no host ports;
- no Docker socket;
- no added Linux capabilities;
- `no-new-privileges`;
- read-only root filesystem;
- tmpfs at `/tmp`;
- persistent bind mount at `/data`;
- container process runs as the unprivileged `home-ai-core` UID/GID;
- persistent host data lives at `/var/lib/home-ai-modules/<module-id>`.

A future manifest revision may add explicitly reviewed declarations for ports, devices or extra mounts. v2 intentionally does not allow them.

## Reserved upstream profiles

Core 0.1.209 adds one explicitly reviewed exception: `homeassistant.module` with
an immutable image from `ghcr.io/home-assistant/home-assistant` only. Its fixed
upstream profile uses host networking, privileged device access, persistent
`/config` generations and a fixed HTTP health check. The manifest must declare
`host.network` and `host.privileged`, and the UI requests separate installation
confirmation. These permission strings do not grant host access to other IDs.
No generic manifest fields for shell commands, arbitrary devices/mounts, host
networking or privilege escalation have been added.

See [Home Assistant Container](../integrations/homeassistant-container.md) for
its storage/backup/rollback contract and the limits of privileged isolation.

## Lifecycle

Mutation requests are submitted to the Core Job Engine.

Supported operations in this slice:

- install — pull immutable image, create container and start it;
- start / enable;
- stop / disable;
- restart;
- remove — remove the container while preserving `/data`.

The network-facing Core never receives `docker.sock`. Jobs call the root helper over the existing authenticated Unix socket. The helper validates module ID, image digest, managed labels and container ownership before invoking Docker.

## Capabilities

Every Docker module must require:

```text
host.docker
```

Core also exposes host architecture and other generic capabilities. Module-provided capabilities contribute only while a module is enabled.

## API and UI

Module installation can be submitted through the Web UI by pasting a trusted Manifest v2.

The REST API remains under `/api/v1/modules`. See `../api/modules-v1.md`.

Executable JavaScript is not accepted from a manifest. UI contribution remains declarative navigation metadata only.

## Reserved Frigate profile (Core 0.1.211+)

`frigate.module` is another narrowly reviewed exception, not a configurable
privilege mechanism in Manifest v2. Only official `ghcr.io/blakeblackshear/frigate`
platform digests and the supported version range are accepted. The upstream
container uses root and a writable image layer, fixed minimum capabilities,
separate `home-ai-frigate` network and managed `/config` and `/media/frigate`
mounts. Host ports, devices/GPU, privileged mode and docker.sock are forbidden.
Core 0.1.213+ serves the original Frigate application through a separate, authenticated HTTPS origin (default :8971), never in Core's origin. Every request requires modules.manage; Core credentials and upstream cookies do not cross this boundary. The earlier administrative API bridge remains for compatibility. No manifest can select an arbitrary UI host, port or executable code.
Updates include a verified stopped-container configuration/SQLite snapshot,
transaction journal, manual approval and rollback. See `../integrations/frigate.md`.


## Installed-module sidebar contract

Installed registry records own sidebar entries, not the module catalog or the
container running state. Missing/empty `ui.navigation` receives a standard route
`/modules/<module-id>` with the module name. Existing stored manifests are supported
without rewriting them or reinstalling their containers. Declared routes remain
scoped to that module, are deduplicated and obey module permissions.

A stopped/error module keeps its entry and opens management; removal removes the
entry. Successful lifecycle operations refresh navigation without a page reload.
The frontend coalesces events during an in-flight read, polls as a fallback,
refreshes on focus/reconnect and cancels reads from a previous login session.
The browser regression suite covers this contract using the production bundle.
