# Network Management API v3

## Scope

Network Management v3 adds safe persistent support for Debian `ifupdown` while keeping the NetworkManager, systemd-networkd, runtime network controls and WireGuard support from v2.

The key design rule is ownership: **Home-AI never silently overwrites an existing administrator-managed ifupdown stanza.**

## ifupdown discovery

When `/etc/network/interfaces` is the active backend, Home-AI reads:

- `/etc/network/interfaces`;
- active includes under `/etc/network/interfaces.d/` when the main file contains a matching `source` or `source-directory` directive.

For IPv4 `iface <name> inet ...` stanzas Home-AI extracts:

- method: DHCP/static/other;
- address;
- netmask;
- gateway;
- `dns-nameservers`;
- source file.

An IPv4 address written as separate `address` + `netmask` is normalized to CIDR for the API/Web UI where the netmask is valid.

## Ownership model

Every reported profile has an ownership state:

- `none` — no persistent IPv4 stanza is active;
- `home-ai` — the stanza lives in a Home-AI-owned file and contains the Home-AI management marker;
- `external` — the stanza comes from an administrator/vendor-managed file;
- `conflict` — multiple active IPv4 stanzas exist for the same interface.

### External profiles

External profiles are shown in the UI, including their source and parsed settings, but are read-only.

A `profile.save` request for an externally managed interface is rejected.

There is intentionally no implicit takeover operation.

### Conflicts

When multiple active IPv4 stanzas exist for one interface, Home-AI reports the conflict and refuses to modify the interface persistently until the conflict is resolved manually.

## Home-AI-owned ifupdown files

Home-AI writes one file per managed interface:

```text
/etc/network/interfaces.d/50-home-ai-<interface>
```

Each file starts with:

```text
# Managed by Home-AI-Core
# This file is owned by Home-AI-Core.
```

DHCP example:

```text
# Managed by Home-AI-Core
# This file is owned by Home-AI-Core.
auto eno2
iface eno2 inet dhcp
    dns-nameservers 1.1.1.1 8.8.8.8
```

Static example:

```text
# Managed by Home-AI-Core
# This file is owned by Home-AI-Core.
auto eno2
iface eno2 inet static
    address 192.168.50.20/24
    gateway 192.168.50.1
    dns-nameservers 1.1.1.1 8.8.8.8
```

Files are replaced atomically.

## interfaces.d include

If `/etc/network/interfaces` already contains an active `source` or `source-directory` for `/etc/network/interfaces.d`, Home-AI leaves the main file unchanged.

If no such include exists, Home-AI:

1. reads the existing main file;
2. creates a timestamped backup;
3. appends only a clearly marked include block;
4. atomically replaces the main file.

The existing interface stanzas are not rewritten.

The inserted block is:

```text
# BEGIN Home-AI-Core managed include
source /etc/network/interfaces.d/*
# END Home-AI-Core managed include
```

The operation is idempotent.

## Applying a profile

After writing a Home-AI-owned profile, Home-AI applies only the selected interface.

The privileged helper delegates commands through the existing transient `systemd-run` execution path.

If available, `ifquery <interface>` validates that the interface is present in the active ifupdown configuration.

Home-AI then uses:

```text
ifdown --force <interface>
ifup <interface>
```

A failed `ifdown` is non-fatal for a newly managed interface; the subsequent `ifup` is decisive.

Applying a network profile may interrupt the current Web session if the interface carries the connection to Home-AI. The Web UI requires explicit confirmation before saving/applying.

## API compatibility

The helper protocol remains v2.

The existing endpoints remain unchanged:

```text
GET  /api/v1/network/profiles
POST /api/v1/network/operation
```

`profile.save` is now supported when the active backend is ifupdown and the selected interface either:

- has no existing active IPv4 stanza; or
- is already managed by Home-AI.

## Safety rules

- loopback is never offered as a managed persistent profile;
- WireGuard interfaces remain managed by the WireGuard subsystem;
- external ifupdown stanzas are read-only;
- duplicate active stanzas cause a conflict state;
- Home-AI writes only its own per-interface files;
- existing administrator stanzas are not edited;
- the main interfaces file is only changed to add the managed include block when required;
- a backup is created before that include change;
- writes are atomic;
- all mutating API operations still require `network.manage`, CSRF for cookie sessions and Audit logging.

## Deferred

Still not implemented:

- explicit takeover/import of an external ifupdown stanza;
- IPv6 persistent profile editor;
- VLANs;
- bridges;
- bonds;
- Wi-Fi configuration;
- firewall/NAT management;
- WireGuard client enrollment/QR generation.
