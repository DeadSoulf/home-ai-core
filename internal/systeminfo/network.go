package systeminfo

import (
	"bufio"
	"encoding/hex"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

func networkInterfaces(sysClassNet string) []NetworkInterface {
	return networkInterfacesFromPaths(
		sysClassNet,
		"/proc/net/if_inet6",
		interfaceIPv4Address,
	)
}

func networkInterfacesFromPaths(
	sysClassNet string,
	procInet6 string,
	ipv4Address func(string) string,
) []NetworkInterface {
	entries, err := os.ReadDir(sysClassNet)
	if err != nil {
		return []NetworkInterface{}
	}

	ipv6 := ipv6Addresses(procInet6)
	result := make([]NetworkInterface, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && entry.Type()&os.ModeSymlink == 0 {
			continue
		}
		name := entry.Name()
		base := filepath.Join(sysClassNet, name)

		index, _ := strconv.Atoi(readTrimmed(filepath.Join(base, "ifindex")))
		mtu, _ := strconv.Atoi(readTrimmed(filepath.Join(base, "mtu")))
		flags, _ := strconv.ParseUint(readTrimmed(filepath.Join(base, "flags")), 0, 64)
		speedMbps, _ := strconv.ParseUint(readTrimmed(filepath.Join(base, "speed")), 10, 64)

		addresses := make([]string, 0, 1+len(ipv6[name]))
		if address := ipv4Address(name); address != "" {
			addresses = append(addresses, address)
		}
		addresses = append(addresses, ipv6[name]...)
		sort.Strings(addresses)

		result = append(result, NetworkInterface{
			Name:      name,
			Index:     index,
			MAC:       readTrimmed(filepath.Join(base, "address")),
			MTU:       mtu,
			Up:        flags&unix.IFF_UP != 0,
			Loopback:  flags&unix.IFF_LOOPBACK != 0,
			Multicast: flags&unix.IFF_MULTICAST != 0,
			OperState: readTrimmed(filepath.Join(base, "operstate")),
			SpeedBPS:  speedMbps * 1_000_000,
			Addresses: addresses,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

func interfaceIPv4Address(name string) string {
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return ""
	}
	defer unix.Close(fd)

	addressReq, err := unix.NewIfreq(name)
	if err != nil {
		return ""
	}
	if err := unix.IoctlIfreq(fd, unix.SIOCGIFADDR, addressReq); err != nil {
		return ""
	}
	addressBytes, err := addressReq.Inet4Addr()
	if err != nil || len(addressBytes) != net.IPv4len {
		return ""
	}

	maskReq, err := unix.NewIfreq(name)
	if err != nil {
		return net.IP(addressBytes).String()
	}
	if err := unix.IoctlIfreq(fd, unix.SIOCGIFNETMASK, maskReq); err != nil {
		return net.IP(addressBytes).String()
	}
	maskBytes, err := maskReq.Inet4Addr()
	if err != nil || len(maskBytes) != net.IPv4len {
		return net.IP(addressBytes).String()
	}
	ones, bits := net.IPMask(maskBytes).Size()
	if bits != 32 || ones < 0 {
		return net.IP(addressBytes).String()
	}
	return net.IP(addressBytes).String() + "/" + strconv.Itoa(ones)
}

func ipv6Addresses(path string) map[string][]string {
	result := map[string][]string{}

	file, err := os.Open(path)
	if err != nil {
		return result
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 6 || len(fields[0]) != 32 {
			continue
		}
		rawIP, err := hex.DecodeString(fields[0])
		if err != nil || len(rawIP) != net.IPv6len {
			continue
		}
		prefix, err := strconv.ParseUint(fields[2], 16, 8)
		if err != nil {
			continue
		}
		name := fields[5]
		result[name] = append(
			result[name],
			net.IP(rawIP).String()+"/"+strconv.Itoa(int(prefix)),
		)
	}

	for name := range result {
		sort.Strings(result[name])
	}
	return result
}
