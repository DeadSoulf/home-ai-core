package systeminfo

import (
	"net"
	"path/filepath"
	"sort"
	"strconv"
)

func networkInterfaces(sysClassNet string) []NetworkInterface {
	interfaces, err := net.Interfaces()
	if err != nil {
		return []NetworkInterface{}
	}

	result := make([]NetworkInterface, 0, len(interfaces))
	for _, iface := range interfaces {
		addresses := []string{}
		if addrs, err := iface.Addrs(); err == nil {
			for _, addr := range addrs {
				addresses = append(addresses, addr.String())
			}
			sort.Strings(addresses)
		}

		speedMbps, _ := strconv.ParseUint(
			readTrimmed(filepath.Join(sysClassNet, iface.Name, "speed")),
			10,
			64,
		)

		result = append(result, NetworkInterface{
			Name:      iface.Name,
			Index:     iface.Index,
			MAC:       iface.HardwareAddr.String(),
			MTU:       iface.MTU,
			Up:        iface.Flags&net.FlagUp != 0,
			Loopback:  iface.Flags&net.FlagLoopback != 0,
			Multicast: iface.Flags&net.FlagMulticast != 0,
			OperState: readTrimmed(filepath.Join(sysClassNet, iface.Name, "operstate")),
			SpeedBPS:  speedMbps * 1_000_000,
			Addresses: addresses,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}
