# Third-Party Code Policy

Home-AI-Core may study other server-management projects for architecture, workflows and UX.

## Preferred approach

1. Prefer upstream system APIs and libraries.
2. Reimplement architectural ideas in Home-AI-Core-owned code.
3. Reuse third-party source code only when it provides clear engineering value.
4. Record license, origin, copyright and modifications for every reused component.

## Project references

- Cockpit: Linux/Web privilege boundary and package model.
- OpenMediaVault: modular configuration and declarative plugin concepts.
- YunoHost: manifest resources and application lifecycle.
- TrueNAS: jobs, events and storage middleware concepts.
- Proxmox VE: virtualization resource model.
- Portainer CE: container and Compose management UX.
- CasaOS: dashboard and application-store UX.
- Umbrel: consumer-grade app installation UX.
- Cosmos: security gateway concepts.
- Unraid: unified home-server product experience.

## Reuse guidance

Permissively licensed components may be evaluated for direct reuse after file-level license verification.

Copyleft, source-available and non-commercially restricted projects should normally be treated as reference implementations unless the project's licensing strategy explicitly allows their inclusion.

Every direct reuse must be documented in a future `THIRD_PARTY.md` record before merge.
