package systeminfo

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const sectorSize = uint64(512)

type lsblkOutput struct {
	BlockDevices []lsblkNode `json:"blockdevices"`
}

type lsblkNode struct {
	Name        string      `json:"name"`
	Path        string      `json:"path"`
	Type        string      `json:"type"`
	Filesystem  string      `json:"fstype"`
	SizeBytes   uint64      `json:"size"`
	Mountpoints []*string   `json:"mountpoints"`
	ParentName  string      `json:"pkname"`
	Label       string      `json:"label"`
	UUID        string      `json:"uuid"`
	Model       string      `json:"model"`
	Vendor      string      `json:"vendor"`
	Serial      string      `json:"serial"`
	Rotational  bool        `json:"rota"`
	Removable   bool        `json:"rm"`
	Children    []lsblkNode `json:"children"`
}

func lsblkTree() []BlockNode {
	command := exec.Command(
		"/usr/bin/lsblk",
		"--json",
		"--bytes",
		"--output",
		"NAME,PATH,TYPE,FSTYPE,SIZE,MOUNTPOINTS,PKNAME,LABEL,UUID,MODEL,VENDOR,SERIAL,ROTA,RM",
	)
	output, err := command.Output()
	if err != nil {
		return []BlockNode{}
	}

	var decoded lsblkOutput
	if err := json.Unmarshal(output, &decoded); err != nil {
		return []BlockNode{}
	}

	result := make([]BlockNode, 0, len(decoded.BlockDevices))
	for _, item := range decoded.BlockDevices {
		if ignoredLsblkType(item) {
			continue
		}
		result = append(result, convertLsblkNode(item))
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func ignoredLsblkType(item lsblkNode) bool {
	if item.Type == "loop" {
		return true
	}
	return ignoredBlockDevice(item.Name)
}

func convertLsblkNode(item lsblkNode) BlockNode {
	mountpoints := make([]string, 0, len(item.Mountpoints))
	system := false
	for _, rawMountpoint := range item.Mountpoints {
		if rawMountpoint == nil {
			continue
		}
		mountpoint := strings.TrimSpace(*rawMountpoint)
		if mountpoint == "" {
			continue
		}
		mountpoints = append(mountpoints, mountpoint)
		if mountpoint == "/" {
			system = true
		}
	}

	children := make([]BlockNode, 0, len(item.Children))
	for _, child := range item.Children {
		node := convertLsblkNode(child)
		if node.System {
			system = true
		}
		children = append(children, node)
	}

	return BlockNode{
		Name:        item.Name,
		Path:        item.Path,
		Type:        item.Type,
		Filesystem:  item.Filesystem,
		SizeBytes:   item.SizeBytes,
		Mountpoints: mountpoints,
		ParentName:  item.ParentName,
		Label:       item.Label,
		UUID:        item.UUID,
		Model:       strings.TrimSpace(item.Model),
		Vendor:      strings.TrimSpace(item.Vendor),
		Serial:      strings.TrimSpace(item.Serial),
		Rotational:  item.Rotational,
		Removable:   item.Removable,
		System:      system,
		Children:    children,
	}
}

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
		device.System = mountedAtRoot(mounts[device.Path])
		if !device.System {
			for _, part := range device.Partitions {
				if mountedAtRoot(mounts[part.Path]) {
					device.System = true
					break
				}
			}
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
		props := udevBlockProperties(readTrimmed(filepath.Join(partPath, "dev")))
		p := Partition{
			Name:        name,
			Path:        devicePath,
			SizeBytes:   readUint(filepath.Join(partPath, "size")) * sectorSize,
			Filesystem:  props["ID_FS_TYPE"],
			UUID:        props["ID_FS_UUID"],
			Label:       props["ID_FS_LABEL"],
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

func mountedAtRoot(items []mountInfo) bool {
	for _, item := range items {
		if item.Mountpoint == "/" {
			return true
		}
	}
	return false
}

func udevBlockProperties(majorMinor string) map[string]string {
	result := map[string]string{}
	if majorMinor == "" {
		return result
	}
	file, err := os.Open(filepath.Join("/run/udev/data", "b"+majorMinor))
	if err != nil {
		return result
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "E:") {
			continue
		}
		key, value, ok := strings.Cut(strings.TrimPrefix(line, "E:"), "=")
		if ok {
			result[key] = value
		}
	}
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
