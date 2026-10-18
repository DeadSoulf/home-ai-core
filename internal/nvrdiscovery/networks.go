// Package nvrdiscovery searches for local video devices using multiple
// independent mechanisms. A discovered host is not proof that its stream
// is usable: credentials and a vendor-specific RTSP path may still be needed.
package nvrdiscovery

import (
	"encoding/binary"
	"errors"
	"net/netip"
	"sort"
	"strings"

	"github.com/DeadSoulf/home-ai-core/internal/systeminfo"
)

const maxNetworkHosts = 254

type Network struct {
	ID        string `json:"id"`
	Interface string `json:"interface"`
	Address   string `json:"address"`
	CIDR      string `json:"cidr"`
	Hosts     int    `json:"hosts"`
}

func subnetForAddress(addr netip.Addr, maskBits int) (netip.Prefix, bool) {
	if !addr.Is4() || !addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() ||
		maskBits < 0 || maskBits > 30 {
		return netip.Prefix{}, false
	}
	// A /16 on a home interface must not silently trigger a large scan.
	// Inspect at most the /24 containing that interface address.
	if maskBits < 24 {
		maskBits = 24
	}
	return netip.PrefixFrom(addr, maskBits).Masked(), true
}

func excludedInterface(name string) bool {
	name = strings.ToLower(name)
	for _, prefix := range []string{"docker", "veth", "br-", "virbr", "podman", "cni", "flannel"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// Networks enumerates directly connected private IPv4 interfaces without
// AF_NETLINK, which is intentionally denied by the hardened Core service.
// No browser-supplied CIDR can influence this inventory.
func Networks() ([]Network, error) {
	interfaces, err := systeminfo.HostNetworkInterfaces()
	if err != nil {
		return nil, err
	}
	return networksFromHostInterfaces(interfaces), nil
}

// Kept separate from the OS inventory so policy can be tested without network
// privileges. Only private, currently active, directly connected addresses are
// eligible; wide networks are bounded to the host's /24.
func networksFromHostInterfaces(interfaces []systeminfo.NetworkInterface) []Network {
	found := make([]Network, 0, 8)
	seen := make(map[string]bool)
	for _, iface := range interfaces {
		if !iface.Up || iface.Loopback || excludedInterface(iface.Name) {
			continue
		}
		for _, rawAddress := range iface.Addresses {
			address, err := netip.ParsePrefix(rawAddress)
			if err != nil {
				continue
			}
			ip := address.Addr().Unmap()
			prefix, ok := subnetForAddress(ip, address.Bits())
			if !ok {
				continue
			}
			hosts := (1 << (32 - prefix.Bits())) - 3 // network, broadcast and this Core host
			if hosts < 1 || hosts > maxNetworkHosts {
				continue
			}
			id := iface.Name + "|" + prefix.String()
			if seen[id] {
				continue
			}
			seen[id] = true
			found = append(found, Network{
				ID: id, Interface: iface.Name, Address: ip.String(),
				CIDR: prefix.String(), Hosts: hosts,
			})
		}
	}
	sort.Slice(found, func(i, j int) bool {
		if found[i].Interface != found[j].Interface {
			return found[i].Interface < found[j].Interface
		}
		return found[i].CIDR < found[j].CIDR
	})
	if len(found) > 24 {
		found = found[:24]
	}
	return found
}

func SelectNetwork(available []Network, id string) (Network, error) {
	for _, network := range available {
		if id != "" && network.ID == id {
			return network, nil
		}
	}
	return Network{}, errors.New("select a directly connected private network from the list")
}

func hostsForNetwork(network Network) ([]string, error) {
	prefix, err := netip.ParsePrefix(network.CIDR)
	if err != nil || !prefix.IsValid() || !prefix.Addr().Is4() ||
		prefix.Bits() < 24 || prefix.Bits() > 30 || !prefix.Addr().IsPrivate() {
		return nil, errors.New("unsafe camera discovery network")
	}
	prefix = prefix.Masked()
	base := binary.BigEndian.Uint32(prefix.Addr().AsSlice())
	count := uint32(1 << (32 - prefix.Bits()))
	hosts := make([]string, 0, count-2)
	for offset := uint32(1); offset < count-1; offset++ {
		var raw [4]byte
		binary.BigEndian.PutUint32(raw[:], base+offset)
		addr := netip.AddrFrom4(raw)
		if addr.String() != network.Address {
			hosts = append(hosts, addr.String())
		}
	}
	return hosts, nil
}
