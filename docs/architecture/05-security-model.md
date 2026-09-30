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

## Human identity and profiles

Home-AI-Core owns one canonical human account per person. NAS, NVR, Smart Home,
AI and network-facing adapters must reference that Core user ID instead of
creating independent application users.

Household profile names are permission templates, not hard-coded authorization
shortcuts. The supported templates are Administrator, Parent, Child, Friend and
Guest. The bootstrap owner is a protected Administrator-equivalent identity.

For non-administrators the effective global permission set may differ from the
template through explicit per-user allow/deny overrides. Resource access uses
the same exact scoped-grant contract described in ADR-0014. The first editable
resource catalog is NAS `file_folder`; rooms, devices and cameras can be added
without changing the identity model.

Administrators are dynamically authorized for the complete permission catalog
so newly introduced Core/module permissions do not accidentally leave an
administrator partially privileged.
