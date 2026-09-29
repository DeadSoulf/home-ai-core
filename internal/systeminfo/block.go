package systeminfo

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const sectorSize = uint64(512)

type filesystemFreeCacheEntry struct {
	bytes     uint64
	expiresAt time.Time
}

var filesystemFreeCache = struct {
	sync.Mutex
	items map[string]filesystemFreeCacheEntry
}{items: map[string]filesystemFreeCacheEntry{}}

type lsblkOutput struct {
	BlockDevices []lsblkNode `json:"blockdevices"`
}

type lsblkNode struct {
	Name        string      `json:"name"`
	Path        string      `json:"path"`
	Type        string      `json:"type"`
	Filesystem    string      `json:"fstype"`
	PartitionTable string      `json:"pttype"`
	SizeBytes     uint64      `json:"size"`
	FreeBytes   uint64      `json:"fsavail"`
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
		"NAME,PATH,TYPE,FSTYPE,PTTYPE,SIZE,FSAVAIL,MOUNTPOINTS,PKNAME,LABEL,UUID,MODEL,VENDOR,SERIAL,ROTA,RM",
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

	freeBytes := item.FreeBytes
	var unallocatedBytes uint64
	if freeBytes == 0 && len(mountpoints) == 0 && item.Path != "" && item.Filesystem != "" {
		freeBytes = offlineFilesystemFreeBytes(item.Path, item.Filesystem)
	}
	if item.Type == "disk" {
		var partitionBytes uint64
		for _, child := range children {
			if child.Type == "part" {
				partitionBytes += child.SizeBytes
			}
		}
		if item.SizeBytes > partitionBytes {
			unallocatedBytes = item.SizeBytes - partitionBytes
			freeBytes += unallocatedBytes
		}
	}

	return BlockNode{
		Name:        item.Name,
		Path:        item.Path,
		Type:        item.Type,
		Filesystem:  item.Filesystem,
		SizeBytes:   item.SizeBytes,
		FreeBytes:        freeBytes,
		UnallocatedBytes: unallocatedBytes,
		PartitionTable:   strings.TrimSpace(item.PartitionTable),
		Mountpoints:      mountpoints,
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

func offlineFilesystemFreeBytes(device, filesystem string) uint64 {
	key := device + "|" + strings.ToLower(strings.TrimSpace(filesystem))
	now := time.Now()

	filesystemFreeCache.Lock()
	if cached, ok := filesystemFreeCache.items[key]; ok && now.Before(cached.expiresAt) {
		filesystemFreeCache.Unlock()
		return cached.bytes
	}
	filesystemFreeCache.Unlock()

	var value uint64
	switch strings.ToLower(strings.TrimSpace(filesystem)) {
	case "ext2", "ext3", "ext4":
		value = extFilesystemFreeBytes(device)
	case "xfs":
		value = xfsFilesystemFreeBytes(device)
	case "vfat", "fat", "fat32":
		value = fatFilesystemFreeBytes(device)
	}

	filesystemFreeCache.Lock()
	filesystemFreeCache.items[key] = filesystemFreeCacheEntry{
		bytes:     value,
		expiresAt: now.Add(30 * time.Second),
	}
	filesystemFreeCache.Unlock()
	return value
}

func extFilesystemFreeBytes(device string) uint64 {
	output, err := exec.Command("/usr/sbin/dumpe2fs", "-h", device).CombinedOutput()
	if err != nil {
		return 0
	}
	var freeBlocks, blockSize uint64
	for _, line := range strings.Split(string(output), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		number, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
		if err != nil {
			continue
		}
		switch strings.TrimSpace(key) {
		case "Free blocks":
			freeBlocks = number
		case "Block size":
			blockSize = number
		}
	}
	if freeBlocks == 0 || blockSize == 0 {
		return 0
	}
	return freeBlocks * blockSize
}

func fatFilesystemFreeBytes(device string) uint64 {
	output, err := exec.Command("/usr/sbin/fsck.fat", "-n", "-v", device).CombinedOutput()
	if err != nil {
		return 0
	}
	var clusterSize uint64
	var usedClusters, totalClusters uint64
	for _, line := range strings.Split(string(output), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "bytes per cluster") {
			fields := strings.Fields(trimmed)
			if len(fields) > 0 {
				clusterSize, _ = strconv.ParseUint(fields[0], 10, 64)
			}
		}
		if !strings.Contains(trimmed, "clusters") || !strings.Contains(trimmed, "/") {
			continue
		}
		for _, field := range strings.Fields(trimmed) {
			if !strings.Contains(field, "/") {
				continue
			}
			parts := strings.SplitN(strings.Trim(field, ",;()"), "/", 2)
			if len(parts) != 2 {
				continue
			}
			used, errUsed := strconv.ParseUint(parts[0], 10, 64)
			total, errTotal := strconv.ParseUint(parts[1], 10, 64)
			if errUsed == nil && errTotal == nil && total >= used {
				usedClusters = used
				totalClusters = total
			}
		}
	}
	if clusterSize == 0 || totalClusters == 0 || totalClusters < usedClusters {
		return 0
	}
	return (totalClusters - usedClusters) * clusterSize
}

func xfsFilesystemFreeBytes(device string) uint64 {
	output, err := exec.Command(
		"/usr/sbin/xfs_db",
		"-r",
		"-c", "sb 0",
		"-c", "p blocksize fdblocks",
		device,
	).CombinedOutput()
	if err != nil {
		return 0
	}
	var freeBlocks, blockSize uint64
	for _, line := range strings.Split(string(output), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		number, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
		if err != nil {
			continue
		}
		switch strings.TrimSpace(key) {
		case "fdblocks":
			freeBlocks = number
		case "blocksize":
			blockSize = number
		}
	}
	if freeBlocks == 0 || blockSize == 0 {
		return 0
	}
	return freeBlocks * blockSize
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
