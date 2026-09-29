# Network Management API v2

## Scope

Network Management v2 extends v1 with persistent IPv4 profiles while preserving the low-level runtime tools and WireGuard management.

The privileged helper remains sandboxed. Commands that require netlink, D-Bus or unrestricted network socket families are delegated to short-lived transient systemd services through `systemd-run`.

This keeps the long-running root helper restricted while allowing explicit network-management actions to work correctly.

## Permissions

- `network.read` — read network profile and WireGuard status.
- `network.manage` — change runtime network settings, persistent profiles and WireGuard.

The built-in `owner` role receives both permissions.

## Runtime operations

```text
POST /api/v1/network/operation
```

Runtime operations remain available:

- `link.up`
- `link.down`
- `mtu`
- `address.add`
- `address.delete`
- `gateway.set`
- `gateway.delete`

These are immediate Linux runtime operations.

## Persistent profile discovery

```text
GET /api/v1/network/profiles
```

Requires `network.read`.

The response includes:

- detected backend;
- interface;
- whether persistent management is supported;
- whether a profile currently exists;
- method (`dhcp` or `static`);
- static CIDR address;
- default gateway;
- DNS servers;
- source connection/profile.

Supported writable backends in v2:

- NetworkManager;
- systemd-networkd.

Detected but read-only in v2:

- ifupdown.

Unknown backends are never rewritten automatically.

## Save persistent profile

Use:

```text
POST /api/v1/network/operation
```

with:

```json
{
  "operation": "profile.save",
  "interface": "enp3s0",
  "network_method": "static",
  "address": "192.168.50.10/24",
  "gateway": "192.168.50.1",
  "dns": ["1.1.1.1", "8.8.8.8"]
}
```

DHCP example:

```json
{
  "operation": "profile.save",
  "interface": "enp3s0",
  "network_method": "dhcp",
  "dns": []
}
```

Applying a profile can change the server address and interrupt the current Web connection. The Web UI requires explicit confirmation.

## NetworkManager

Home-AI uses the active Ethernet connection for the interface.

If an Ethernet interface has no connection profile, Home-AI creates:

```text
home-ai-<interface>
```

Static profiles use NetworkManager IPv4 manual addressing.

DHCP profiles use NetworkManager IPv4 automatic addressing.

When explicit DNS servers are supplied, automatic DHCP DNS is ignored for IPv4. When the DNS list is empty, automatic DNS is enabled.

The connection is brought up immediately after modification.

## systemd-networkd

Home-AI writes an owned profile:

```text
/etc/systemd/network/05-home-ai-<interface>.network
```

The early filename makes the explicitly configured Home-AI profile the first matching networkd profile for that interface.

After an atomic write, Home-AI runs:

```text
networkctl reload
networkctl reconfigure <interface>
```

Only the Home-AI-owned file is written. Existing distribution/operator profiles are not edited.

## ifupdown

Home-AI detects an `/etc/network/interfaces` based setup but does not rewrite it in v2.

This is intentional: Debian installations can contain hand-written stanzas, includes, bridges and other custom directives. Editing those safely requires a dedicated parser/ownership model.

The Web UI reports `ifupdown` as the active backend and marks persistent editing unsupported rather than risking network loss.

Runtime `ip` operations remain separate.

## WireGuard

WireGuard functionality from v1 remains available:

- install `wireguard-tools` by explicit owner action;
- create/delete tunnel;
- start/stop tunnel;
- persistent `/etc/wireguard/<name>.conf`;
- peer add/remove;
- endpoint;
- allowed IPs;
- persistent keepalive;
- handshake and traffic statistics.

APT installation is delegated through a transient systemd service so APT can drop privileges to `_apt` and use Internet sockets without weakening the updater-helper service.

WireGuard and `ip` commands are also executed through transient units when they need kernel networking APIs unavailable inside the helper sandbox.

## Validation and safety

- loopback cannot be managed;
- interface names are validated;
- static addresses require CIDR notation;
- gateways must be valid IP addresses;
- static address and gateway must use the same IP family;
- DNS entries must be IP addresses;
- duplicate DNS entries are removed;
- WireGuard key material is validated and private keys are never returned;
- all mutating API calls require `network.manage`;
- cookie-authenticated writes require CSRF;
- Web UI confirms potentially disconnecting profile changes;
- successful network mutations are audited.

## Deferred

Not yet included:

- safe persistent ifupdown editing;
- IPv6 profile editor;
- VLAN creation;
- bridges/bonds;
- Wi-Fi credential management;
- firewall/NAT policy;
- WireGuard client enrollment/QR generation.
