package systeminfo

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const sectorSize = uint64(512)

func blockDevices(sysBlockRoot string) []BlockDevice {
	entries, err := os.ReadDir(sysBlockRoot)
	if err != nil {
		return []BlockDevice{}
	}

	devices := make([]BlockDevice, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() && ignoredBlockDevice(entry.Name()) {
			continue
		}
		if ignoredBlockDevice(entry.Name()) {
			continue
		}

		base := filepath.Join(sysBlockRoot, entry.Name())
		sectors := readUint(filepath.Join(base, "size"))

		device := BlockDevice{
			Name:       entry.Name(),
			Path:       filepath.Join("/dev", entry.Name()),
			MajorMinor: readTrimmed(filepath.Join(base, "dev")),
			Model:      readTrimmed(filepath.Join(base, "device", "model")),
			Serial:     readTrimmed(filepath.Join(base, "device", "serial")),
			SizeBytes:  sectors * sectorSize,
			Rotational: readBool01(filepath.Join(base, "queue", "rotational")),
			Removable:  readBool01(filepath.Join(base, "removable")),
		}
		devices = append(devices, device)
	}

	sort.Slice(devices, func(i, j int) bool {
		return devices[i].Name < devices[j].Name
	})
	return devices
}

func ignoredBlockDevice(name string) bool {
	for _, prefix := range []string{"loop", "ram", "zram", "fd"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}
