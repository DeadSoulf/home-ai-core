package systeminfo

import (
	"bufio"
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

	mounts := mountedFilesystems("/proc/self/mountinfo")
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
			Vendor:     readTrimmed(filepath.Join(base, "device", "vendor")),
			Model:      readTrimmed(filepath.Join(base, "device", "model")),
			Serial:     readTrimmed(filepath.Join(base, "device", "serial")),
			SizeBytes:  sectors * sectorSize,
			Rotational: readBool01(filepath.Join(base, "queue", "rotational")),
			Removable:  readBool01(filepath.Join(base, "removable")),
			Partitions: partitions(base, mounts),
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


type mountInfo struct {
	Filesystem string
	Mountpoint string
}

func partitions(diskPath string, mounts map[string][]mountInfo) []Partition {
	entries, err := os.ReadDir(diskPath)
	if err != nil {
		return []Partition{}
	}
	result := []Partition{}
	for _, entry := range entries {
		name := entry.Name()
		partPath := filepath.Join(diskPath, name)
		if readTrimmed(filepath.Join(partPath, "partition")) == "" {
			continue
		}
		devicePath := filepath.Join("/dev", name)
		p := Partition{
			Name: name,
			Path: devicePath,
			SizeBytes: readUint(filepath.Join(partPath, "size")) * sectorSize,
			Mountpoints: []string{},
		}
		for _, info := range mounts[devicePath] {
			if p.Filesystem == "" {
				p.Filesystem = info.Filesystem
			}
			p.Mountpoints = append(p.Mountpoints, info.Mountpoint)
		}
		sort.Strings(p.Mountpoints)
		result = append(result, p)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func mountedFilesystems(path string) map[string][]mountInfo {
	result := map[string][]mountInfo{}
	file, err := os.Open(path)
	if err != nil {
		return result
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		sep := -1
		for i, field := range fields {
			if field == "-" {
				sep = i
				break
			}
		}
		if sep < 0 || len(fields) <= sep+2 || len(fields) <= 4 {
			continue
		}
		source := fields[sep+2]
		if !strings.HasPrefix(source, "/dev/") {
			continue
		}
		result[source] = append(result[source], mountInfo{
			Filesystem: fields[sep+1],
			Mountpoint: strings.ReplaceAll(fields[4], "\\040", " "),
		})
	}
	return result
}
