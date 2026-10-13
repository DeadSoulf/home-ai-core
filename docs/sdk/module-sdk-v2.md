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
- persistent host data lives at `/var/lib/home-ai-core/modules/<module-id>`.

A future manifest revision may add explicitly reviewed declarations for ports, devices or extra mounts. v2 intentionally does not allow them.

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
