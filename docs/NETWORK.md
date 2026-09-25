# Home AI Core — Network / IPv4

## Purpose

The Network section exposes Linux network interfaces and allows an authorized administrator
to switch an interface between DHCP and static IPv4 configuration without running the main
Home AI Core process as root.

## Web UI

Open:

```text
Сеть
```

Each interface card shows:

- interface name
- UP / DOWN state
- link carrier state
- current IPv4 addresses
- MAC address
- MTU
- default-route marker
- detected IPv4 mode when it can be determined
- current default gateway
- current upstream IPv4 DNS servers when available

Users with `network.manage` can choose:

```text
Режим IPv4:
  DHCP
  Статический IP
```

Static mode provides:

```text
IP-адрес
Маска сети
Шлюз
Основной DNS
Дополнительный DNS
```

The netmask accepts either dotted notation such as `255.255.255.0` or a prefix such as `24`.

The Web UI warns before applying changes because changing the address of the interface used by
the current Web session can immediately disconnect the browser.

## Validation

Before the privileged helper is called, Home AI Core validates:

- interface-name syntax
- interface existence
- IPv4 address
- contiguous IPv4 netmask / prefix
- gateway address
- gateway belonging to the configured subnet
- primary and secondary DNS addresses

The privileged helper validates the received values again.

## Persistent backends

The helper prefers host-native persistent configuration.

### NetworkManager

When NetworkManager owns the interface, Home AI Core updates the active connection profile with
`nmcli`:

- DHCP: `ipv4.method auto`
- static: `ipv4.method manual`
- static address/prefix
- gateway
- DNS
- automatic DNS disabled for static mode

The profile is reactivated after saving.

### systemd-networkd

When `systemd-networkd` is active, Home AI Core writes a dedicated managed configuration:

```text
/etc/systemd/network/00-home-ai-<interface>.network
```

and applies it with:

```text
networkctl reload
networkctl reconfigure <interface>
```

This configuration survives reboot.

### Debian ifupdown

When `/etc/network/interfaces` and `ifup/ifdown` are available, Home AI Core updates the
existing IPv4 stanza for the selected interface, preserving unrelated interface options where
possible. If no stanza exists, it adds a Home AI managed stanza.

Before replacement, the affected file is copied to:

```text
<interfaces-file>.home-ai.bak
```

The interface is then cycled with `ifdown --force` and `ifup`.

### Runtime fallback

If none of the persistent backends is available:

- DHCP falls back to `dhclient` or `udhcpc`
- static IPv4 falls back to `ip addr`, `ip route`, and `resolvectl` when available

The helper explicitly reports that this fallback is runtime-only.

## Privileged Network Helper

The main server remains unprivileged. Network changes are delegated to:

```text
/usr/local/libexec/home-ai-network-helper
```

After building a new helper version, install or refresh it with:

```bash
sudo sh scripts/install-network-helper.sh texnik
```

The helper:

- accepts only validated `dhcp` and `static` actions
- validates interface names
- rejects loopback
- verifies the interface exists in `/sys/class/net`
- validates static IPv4 values
- executes system utilities directly without a shell
- does not accept arbitrary command text

## API

Read interface inventory:

```text
GET /api/network/interfaces
```

Requires:

```text
network.view
```

Configure IPv4:

```text
POST /api/network/ipv4
```

Common form fields:

```text
interface=ens18
mode=dhcp
```

Static mode:

```text
interface=ens18
mode=static
address=192.168.1.20
netmask=255.255.255.0
gateway=192.168.1.1
dns_primary=1.1.1.1
dns_secondary=8.8.8.8
```

Requires:

```text
network.manage
X-HomeAI-Request: 1
```

The older `POST /api/network/dhcp` route remains available for compatibility.

Every IPv4 configuration request is written to the central security audit.
