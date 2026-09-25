# Home AI Core — Network / DHCP

## Purpose

The Network section exposes Linux network interfaces and allows an authorized administrator to request an IPv4 address from a DHCP server without running the main Home AI Core process as root.

## Web UI

Open:

```text
Сеть
```

The interface cards show:

- interface name
- UP / DOWN state
- link carrier state
- current IPv4 addresses
- MAC address
- MTU
- whether the interface owns the default route

Users with `network.manage` see:

```text
Получить IP по DHCP
```

The Web UI warns that the current Web connection can be interrupted if the selected interface changes address.

## DHCP backends

The privileged helper tries supported Linux networking backends in this order:

1. NetworkManager via `nmcli`
2. systemd-networkd via `networkctl renew`
3. `dhclient`
4. `udhcpc`

When NetworkManager owns the interface, Home AI Core changes the active connection profile to `ipv4.method auto`, clears the manual IPv4 address/gateway fields and reactivates it. This makes DHCP persistent in that NetworkManager profile.

For systemd-networkd, `networkctl renew` renews DHCP when DHCP is already enabled in the system network configuration.

With `dhclient` or `udhcpc`, Home AI Core requests an address for the current runtime session. Persistence after reboot remains the responsibility of the host network configuration.

## Privileged Network Helper

The main server remains unprivileged. DHCP changes are delegated to:

```text
/usr/local/libexec/home-ai-network-helper
```

After building the new version, install the helper once:

```bash
sudo sh scripts/install-network-helper.sh texnik
```

The installer creates a narrowly scoped sudo rule for only the validated Home AI network helper.

The helper:

- accepts only the `dhcp` action
- validates the interface name
- rejects loopback
- verifies the interface exists in `/sys/class/net`
- executes networking tools directly without a shell
- does not accept arbitrary commands

## API

Read interface inventory:

```text
GET /api/network/interfaces
```

Requires:

```text
network.view
```

Request DHCP:

```text
POST /api/network/dhcp
```

Form field:

```text
interface=ens18
```

Requires:

```text
network.manage
X-HomeAI-Request: 1
```

Every DHCP request is written to the central security audit.
