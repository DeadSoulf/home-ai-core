# Security Model

## Trust boundaries

```text
Browser
  |
  v
Unprivileged Web/API layer
  |
  v
Authorization / Policy Engine
  |
  v
Privileged Operations Boundary
  |
  +-- systemd / D-Bus
  +-- filesystem
  +-- networking
  +-- package manager
  +-- device access
```

The browser and frontend never execute privileged host commands directly.

## Principles

1. Deny by default.
2. Every privileged action is authenticated and authorized.
3. Modules declare permissions before installation.
4. Permissions are scoped to the smallest practical resource.
5. Secrets never appear in normal logs or API responses.
6. Security-sensitive mutations are written to the audit log.
7. Module installation and updates require integrity verification.
8. External network exposure is explicit, not automatic.

## Permission examples

- `system.services.read`
- `system.services.manage`
- `network.bridge.manage`
- `storage.block.read`
- `storage.filesystem.manage`
- `virtualization.vm.manage`
- `containers.engine.manage`
- `filesystem.read:/srv/media`
- `filesystem.write:/srv/appdata/<module>`
- `device.use:gpu`

## Module installation

Before installation the Core validates:

- manifest schema
- Core API compatibility
- dependencies
- conflicts
- requested permissions
- package signature/integrity
- available disk space
- required host capabilities

The user must be able to review requested permissions before granting them.

## Privileged execution

Privileged execution is separated from the public Web/API surface. The exact implementation will be selected during Core design, but the public API process must not become a generic root shell.
