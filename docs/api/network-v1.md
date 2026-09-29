# Network Management API v1

## Scope

Network Management v1 adds low-level Linux network operations and WireGuard management to the Home-AI-Core System page.

The privileged work is executed by the root helper over the existing authenticated Unix socket.

## Permissions

- `network.read` — read WireGuard status.
- `network.manage` — change interfaces, routes and WireGuard configuration.

The built-in `owner` role receives both permissions.

The built-in `member` role receives neither permission by default.

## Interface status

Basic interface status remains part of:

```text
GET /api/v1/system
```

The response already includes:

- interface name;
- index;
- MAC;
- MTU;
- operational state;
- current addresses;
- link speed where available.

## Network operations

```text
POST /api/v1/network/operation
```

Requires `network.manage`.

Cookie-authenticated requests require a valid CSRF token.

Supported operations:

- `link.up`
- `link.down`
- `mtu`
- `address.add`
- `address.delete`
- `gateway.set`
- `gateway.delete`

Example:

```json
{
  "operation": "address.add",
  "interface": "enp3s0",
  "address": "192.168.10.20/24"
}
```

### Persistence

Interface link state, MTU, addresses and default-route changes in v1 are runtime Linux changes made through `ip`.

They are intentionally not written into NetworkManager, systemd-networkd or ifupdown profiles yet.

A later network-profile layer will detect/own the active Debian network backend and provide persistent DHCP/static-IP/DNS configuration.

This separation avoids corrupting an externally managed network configuration.

## WireGuard status

```text
GET /api/v1/network/wireguard
```

Requires `network.read`.

The response reports:

- tool availability;
- configured tunnels;
- active state;
- tunnel address;
- public key;
- listen port;
- peers;
- endpoint;
- allowed IPs;
- latest handshake;
- RX/TX counters;
- persistent keepalive.

Private keys and preshared keys are never returned by the API.

## WireGuard operations

The same network operation endpoint supports:

- `wireguard.install`
- `wireguard.create`
- `wireguard.up`
- `wireguard.down`
- `wireguard.delete`
- `wireguard.peer.add`
- `wireguard.peer.delete`

### Install

`wireguard.install` explicitly runs the Debian package-manager installation for `wireguard-tools`.

The privileged helper itself stays inside its restricted systemd sandbox. Package-manager work is delegated through `systemd-run` to a short-lived transient root service, because APT must be able to drop privileges to `_apt` and open Internet sockets. The transient service is started only for the explicit install action and is collected after completion.

It is never run automatically during a normal Home-AI-Core update.

New initial installer packages depend on `wireguard-tools` and `iproute2`.

### Tunnel create

Example:

```json
{
  "operation": "wireguard.create",
  "tunnel": "wg0",
  "address": "10.77.0.1/24",
  "listen_port": 51820
}
```

If `private_key` is omitted, the privileged helper generates it locally.

WireGuard configuration is stored under:

```text
/etc/wireguard/<tunnel>.conf
```

with mode `0600`.

Successful tunnels are enabled through `wg-quick@<tunnel>.service` so they survive reboot.

### Peer add

Example:

```json
{
  "operation": "wireguard.peer.add",
  "tunnel": "wg0",
  "peer_public_key": "<base64-public-key>",
  "allowed_ips": ["10.77.0.2/32"],
  "endpoint": "example.net:51820",
  "keepalive": 25
}
```

An optional `preshared_key` is accepted but never returned.

## Safety

- loopback cannot be managed through the interface-operation API;
- interface names are validated;
- IP/CIDR values are validated;
- WireGuard keys must decode to 32 bytes;
- tunnel names are limited to valid Linux interface names;
- Web UI requires explicit confirmation before disabling an interface;
- network actions are recorded in Audit;
- private WireGuard key material is not added to audit metadata.

## Deferred

Network Management v1 does not yet own:

- DHCP profiles;
- static configuration persistence for physical NICs;
- DNS configuration;
- VLAN creation;
- bridges/bonds;
- firewall/NAT policy;
- automatic WireGuard client profile/QR generation.

Those are separate higher-level network-profile features.
