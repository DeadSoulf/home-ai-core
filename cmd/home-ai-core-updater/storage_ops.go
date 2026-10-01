package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
)

type inspectLsblkOutput struct {
	BlockDevices []inspectLsblkNode `json:"blockdevices"`
}

type inspectLsblkNode struct {
	Path        string             `json:"path"`
	Type        string             `json:"type"`
	Filesystem  string             `json:"fstype"`
	FreeBytes   *uint64            `json:"fsavail"`
	Mountpoints []*string          `json:"mountpoints"`
	Children    []inspectLsblkNode `json:"children"`
}

func inspectFilesystemStats(ctx context.Context) ([]updaterhelper.FilesystemStat, error) {
	output, err := exec.CommandContext(
		ctx,
		"/usr/bin/lsblk",
		"--json",
		"--bytes",
		"--output", "PATH,TYPE,FSTYPE,FSAVAIL,MOUNTPOINTS",
	).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("inspect filesystems: %s", strings.TrimSpace(string(output)))
	}
	var decoded inspectLsblkOutput
	if err := json.Unmarshal(output, &decoded); err != nil {
		return nil, fmt.Errorf("decode filesystem inventory: %w", err)
	}

	stats := make([]updaterhelper.FilesystemStat, 0)
	var visit func(inspectLsblkNode)
	visit = func(node inspectLsblkNode) {
		filesystem := strings.ToLower(strings.TrimSpace(node.Filesystem))
		if node.Path != "" && filesystem != "" && filesystem != "swap" && filesystem != "lvm2_member" {
			stat := updaterhelper.FilesystemStat{
				Device:     node.Path,
				Filesystem: filesystem,
			}
			if node.FreeBytes != nil {
				stat.FreeBytes = *node.FreeBytes
				stat.FreeKnown = true
			} else if free, ok := offlineFilesystemFreeBytesPrivileged(ctx, node.Path, filesystem); ok {
				stat.FreeBytes = free
				stat.FreeKnown = true
			}
			stats = append(stats, stat)
		}
		for _, child := range node.Children {
			visit(child)
		}
	}
	for _, node := range decoded.BlockDevices {
		visit(node)
	}
	return stats, nil
}

func offlineFilesystemFreeBytesPrivileged(ctx context.Context, device, filesystem string) (uint64, bool) {
	switch filesystem {
	case "ext2", "ext3", "ext4":
		output, err := exec.CommandContext(ctx, "/usr/sbin/dumpe2fs", "-h", device).CombinedOutput()
		if err != nil {
			return 0, false
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
		if blockSize == 0 {
			return 0, false
		}
		return freeBlocks * blockSize, true
	case "xfs":
		output, err := exec.CommandContext(
			ctx,
			"/usr/sbin/xfs_db",
			"-r",
			"-c", "sb 0",
			"-c", "p blocksize fdblocks",
			device,
		).CombinedOutput()
		if err != nil {
			return 0, false
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
		if blockSize == 0 {
			return 0, false
		}
		return freeBlocks * blockSize, true
	case "vfat", "fat", "fat32":
		output, err := exec.CommandContext(ctx, "/usr/sbin/fsck.fat", "-n", "-v", device).CombinedOutput()
		if err != nil {
			return 0, false
		}
		var clusterSize, usedClusters, totalClusters uint64
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
			return 0, false
		}
		return (totalClusters - usedClusters) * clusterSize, true
	default:
		return 0, false
	}
}

type smartctlJSON struct {
	SmartStatus *struct {
		Passed bool `json:"passed"`
	} `json:"smart_status"`
	Temperature *struct {
		Current int `json:"current"`
	} `json:"temperature"`
	PowerOnTime *struct {
		Hours uint64 `json:"hours"`
	} `json:"power_on_time"`
	NVMe *struct {
		PercentageUsed int `json:"percentage_used"`
		Temperature    int `json:"temperature"`
	} `json:"nvme_smart_health_information_log"`
	ATAAttributes *struct {
		Table []struct {
			Name  string `json:"name"`
			Value int    `json:"value"`
			Raw   struct {
				Value int64 `json:"value"`
			} `json:"raw"`
		} `json:"table"`
	} `json:"ata_smart_attributes"`
}

type healthLsblkOutput struct {
	BlockDevices []struct {
		Path      string `json:"path"`
		Type      string `json:"type"`
		Transport string `json:"tran"`
	} `json:"blockdevices"`
}

func inspectDiskHealth(ctx context.Context) []updaterhelper.DiskHealthStat {
	output, err := exec.CommandContext(
		ctx,
		"/usr/bin/lsblk",
		"--json",
		"--output", "PATH,TYPE,TRAN",
	).CombinedOutput()
	if err != nil {
		return nil
	}
	var devices healthLsblkOutput
	if json.Unmarshal(output, &devices) != nil {
		return nil
	}

	_, smartErr := os.Stat("/usr/sbin/smartctl")
	result := make([]updaterhelper.DiskHealthStat, 0, len(devices.BlockDevices))
	for _, item := range devices.BlockDevices {
		if item.Type != "disk" || item.Path == "" {
			continue
		}
		stat := updaterhelper.DiskHealthStat{
			Device:    item.Path,
			Transport: strings.ToLower(strings.TrimSpace(item.Transport)),
			Health:    "unknown",
		}
		if smartErr != nil {
			stat.SmartError = "smartctl is not installed"
			result = append(result, stat)
			continue
		}

		smartOutput, commandErr := exec.CommandContext(
			ctx,
			"/usr/sbin/smartctl",
			"-j", "-n", "standby", "-H", "-A",
			item.Path,
		).CombinedOutput()
		var decoded smartctlJSON
		if err := json.Unmarshal(smartOutput, &decoded); err != nil {
			message := strings.TrimSpace(string(smartOutput))
			if message == "" && commandErr != nil {
				message = commandErr.Error()
			}
			if len(message) > 240 {
				message = message[:240]
			}
			stat.SmartError = message
			result = append(result, stat)
			continue
		}

		stat.SmartAvailable =
			decoded.SmartStatus != nil ||
				decoded.Temperature != nil ||
				decoded.PowerOnTime != nil ||
				decoded.NVMe != nil ||
				decoded.ATAAttributes != nil
		if decoded.SmartStatus != nil {
			if decoded.SmartStatus.Passed {
				stat.Health = "ok"
			} else {
				stat.Health = "failed"
			}
		}
		if decoded.Temperature != nil && decoded.Temperature.Current > -100 && decoded.Temperature.Current < 200 {
			value := decoded.Temperature.Current
			stat.TemperatureC = &value
		}
		if decoded.PowerOnTime != nil {
			value := decoded.PowerOnTime.Hours
			stat.PowerOnHours = &value
		}
		if decoded.NVMe != nil {
			if stat.TemperatureC == nil && decoded.NVMe.Temperature > -100 && decoded.NVMe.Temperature < 200 {
				value := decoded.NVMe.Temperature
				stat.TemperatureC = &value
			}
			remaining := 100 - decoded.NVMe.PercentageUsed
			if remaining < 0 {
				remaining = 0
			}
			if remaining > 100 {
				remaining = 100
			}
			stat.LifeRemainingPct = &remaining
		}
		if stat.LifeRemainingPct == nil && decoded.ATAAttributes != nil {
			for _, attribute := range decoded.ATAAttributes.Table {
				name := strings.ToLower(strings.ReplaceAll(attribute.Name, "-", "_"))
				switch name {
				case "ssd_life_left", "percent_lifetime_remain", "media_wearout_indicator":
					remaining := attribute.Value
					if remaining < 0 {
						remaining = 0
					}
					if remaining > 100 {
						remaining = 100
					}
					stat.LifeRemainingPct = &remaining
				}
				if stat.LifeRemainingPct != nil {
					break
				}
			}
		}
		if stat.Health == "ok" {
			hot := stat.TemperatureC != nil && *stat.TemperatureC >= 60
			worn := stat.LifeRemainingPct != nil && *stat.LifeRemainingPct <= 10
			if hot || worn {
				stat.Health = "warning"
			}
		}
		if commandErr != nil && stat.Health == "unknown" {
			stat.SmartError = "smartctl could not read SMART data for this device"
			stat.SmartAvailable = false
		}
		result = append(result, stat)
	}
	return result
}

type lvsJSON struct {
	Report []struct {
		LV []struct {
			Path            string `json:"lv_path"`
			LVName          string `json:"lv_name"`
			VGName          string `json:"vg_name"`
			Size            string `json:"lv_size"`
			VGSize          string `json:"vg_size"`
			VGFree          string `json:"vg_free"`
			Attr            string `json:"lv_attr"`
			DataPercent     string `json:"data_percent"`
			MetadataPercent string `json:"metadata_percent"`
		} `json:"lv"`
	} `json:"report"`
}

func inspectLVM(ctx context.Context) []updaterhelper.LVMStat {
	if _, err := os.Stat("/usr/sbin/lvs"); err != nil {
		return nil
	}
	output, err := exec.CommandContext(
		ctx,
		"/usr/sbin/lvs",
		"--reportformat", "json",
		"--units", "b",
		"--nosuffix",
		"-o", "lv_path,lv_name,vg_name,lv_size,vg_size,vg_free,lv_attr,data_percent,metadata_percent",
	).CombinedOutput()
	if err != nil {
		return nil
	}
	var decoded lvsJSON
	if json.Unmarshal(output, &decoded) != nil {
		return nil
	}
	var result []updaterhelper.LVMStat
	for _, report := range decoded.Report {
		for _, item := range report.LV {
			size, _ := strconv.ParseFloat(strings.TrimSpace(item.Size), 64)
			vgSize, _ := strconv.ParseFloat(strings.TrimSpace(item.VGSize), 64)
			vgFree, _ := strconv.ParseFloat(strings.TrimSpace(item.VGFree), 64)
			stat := updaterhelper.LVMStat{
				Device:      strings.TrimSpace(item.Path),
				Name:        lvmMapperName(strings.TrimSpace(item.VGName), strings.TrimSpace(item.LVName)),
				VGName:      strings.TrimSpace(item.VGName),
				LVName:      strings.TrimSpace(item.LVName),
				SizeBytes:   uint64(size),
				VGSizeBytes: uint64(vgSize),
				VGFreeBytes: uint64(vgFree),
				Active:      len(item.Attr) > 4 && item.Attr[4] == 'a',
			}
			if value, err := strconv.ParseFloat(strings.TrimSpace(item.DataPercent), 64); err == nil {
				stat.DataPercent = &value
			}
			if value, err := strconv.ParseFloat(strings.TrimSpace(item.MetadataPercent), 64); err == nil {
				stat.MetadataPercent = &value
			}
			result = append(result, stat)
		}
	}
	return result
}

func lvmMapperName(vg, lv string) string {
	escape := func(value string) string { return strings.ReplaceAll(value, "-", "--") }
	if vg == "" || lv == "" {
		return ""
	}
	return escape(vg) + "-" + escape(lv)
}

func performStorageOperation(ctx context.Context, request updaterhelper.Request) (string, error) {
	device, err := validateBlockDevice(request.Device)
	if err != nil {
		return "", err
	}

	switch request.Operation {
	case "storage.mount":
		targets, err := mountedTargets(ctx, device)
		if err != nil {
			return "", err
		}
		if len(targets) != 0 {
			return "", errors.New("device is already mounted")
		}
		target := strings.TrimSpace(request.Mountpoint)
		if target == "" {
			target = filepath.Join("/mnt/home-ai-core", filepath.Base(device))
		}
		target = filepath.Clean(target)
		if target != "/mnt/home-ai-core" && !strings.HasPrefix(target, "/mnt/home-ai-core/") {
			return "", errors.New("mount point must be under /mnt/home-ai-core")
		}
		if err := os.MkdirAll(target, 0o755); err != nil {
			return "", fmt.Errorf("create mount point: %w", err)
		}
		if err := os.Chmod(target, 0o755); err != nil {
			return "", fmt.Errorf("set mount point permissions: %w", err)
		}
		filesystemOutput, err := exec.CommandContext(ctx, "/usr/bin/lsblk", "-ndo", "FSTYPE", device).CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("inspect filesystem before mount: %s", strings.TrimSpace(string(filesystemOutput)))
		}
		filesystem := strings.ToLower(strings.TrimSpace(string(filesystemOutput)))
		quotaOptions := ""
		switch filesystem {
		case "ext4":
			quotaOptions = "usrquota"
			if features, err := exec.CommandContext(ctx, "/usr/sbin/dumpe2fs", "-h", device).CombinedOutput(); err == nil && strings.Contains(string(features), "Project quota inode:") {
				quotaOptions += ",prjquota"
			}
		case "xfs":
			quotaOptions = "uquota,prjquota"
		}

		mountedWithQuota := false
		if quotaOptions != "" {
			mountArgs := []string{"-o", quotaOptions, "--", device, target}
			output, mountErr := exec.CommandContext(ctx, "/usr/bin/mount", mountArgs...).CombinedOutput()
			if mountErr == nil {
				mountedWithQuota = true
			} else {
				plainOutput, plainErr := exec.CommandContext(ctx, "/usr/bin/mount", "--", device, target).CombinedOutput()
				if plainErr != nil {
					quotaMessage := strings.TrimSpace(string(output))
					plainMessage := strings.TrimSpace(string(plainOutput))
					if quotaMessage == "" {
						quotaMessage = mountErr.Error()
					}
					if plainMessage == "" {
						plainMessage = plainErr.Error()
					}
					diagnostic := filesystemMountDiagnostic(ctx, device, filesystem)
					if diagnostic != "" {
						return "", fmt.Errorf("mount device with quota options: %s; retry without quota options: %s; filesystem diagnostic: %s", quotaMessage, plainMessage, diagnostic)
					}
					return "", fmt.Errorf("mount device with quota options: %s; retry without quota options: %s", quotaMessage, plainMessage)
				}
			}
		} else {
			if output, err := exec.CommandContext(ctx, "/usr/bin/mount", "--", device, target).CombinedOutput(); err != nil {
				message := strings.TrimSpace(string(output))
				if message == "" {
					message = err.Error()
				}
				return "", fmt.Errorf("mount device: %s", message)
			}
		}
		targets, err = mountedTargets(ctx, device)
		if err != nil {
			return "", fmt.Errorf("verify mount: %w", err)
		}
		mounted := false
		for _, mountedTarget := range targets {
			if mountedTarget == target {
				mounted = true
				break
			}
		}
		if !mounted {
			return "", errors.New("mount completed but verification did not find the requested mount")
		}
		if quotaOptions != "" && !mountedWithQuota {
			return "device mounted without quota mount options", nil
		}
		return "device mounted", nil

	case "storage.unmount":
		targets, err := mountedTargets(ctx, device)
		if err != nil {
			return "", err
		}
		for _, target := range targets {
			if target == "/" {
				return "", errors.New("refusing to unmount the root filesystem")
			}
		}
		if len(targets) == 0 {
			return "", errors.New("device is not mounted")
		}
		if err := unmountTargets(ctx, device, targets); err != nil {
			return "", err
		}
		return "device unmounted", nil

	case "storage.partition.create":
		if request.Confirm != "CREATE "+device {
			return "", errors.New("partition creation confirmation does not match disk")
		}
		if err := requireDiskType(ctx, device); err != nil {
			return "", err
		}
		protected, err := samePhysicalDiskAsRoot(ctx, device)
		if err != nil {
			return "", err
		}
		if protected {
			return "", errors.New("refusing to change the partition table on the system disk")
		}
		if mounted, err := diskHasMountedDescendants(ctx, device); err != nil {
			return "", err
		} else if mounted {
			return "", errors.New("all filesystems on the disk must be unmounted before changing partitions")
		}
		if request.SizeMiB > 0 && request.SizeMiB < 16 {
			return "", errors.New("partition size must be at least 16 MiB")
		}
		if request.SizeMiB > 0 && request.SizeMiB > 1024*1024*1024 {
			return "", errors.New("partition size is too large")
		}
		if request.SizeMiB > 0 {
			remaining, err := remainingDiskBytes(ctx, device)
			if err != nil {
				return "", err
			}
			const alignmentReserve = uint64(4 * 1024 * 1024)
			requested := request.SizeMiB * 1024 * 1024
			if remaining <= alignmentReserve || requested > remaining-alignmentReserve {
				return "", errors.New("requested partition size exceeds available unallocated space")
			}
		}
		if err := createPartition(ctx, device, request.SizeMiB); err != nil {
			return "", err
		}
		return "partition created", nil

	case "storage.partition.delete":
		if request.Confirm != filepath.Base(device) {
			return "", errors.New("partition deletion confirmation does not match device")
		}
		if err := requirePartitionType(ctx, device); err != nil {
			return "", err
		}
		protected, err := samePhysicalDiskAsRoot(ctx, device)
		if err != nil {
			return "", err
		}
		if protected {
			return "", errors.New("refusing to delete a partition on the system disk")
		}
		if err := prepareDestructiveChange(ctx, device); err != nil {
			return "", err
		}
		if err := deletePartition(ctx, device); err != nil {
			return "", err
		}
		return "partition deleted", nil

	case "storage.partition.delete_all":
		if request.Confirm != filepath.Base(device) {
			return "", errors.New("delete-all confirmation does not match disk")
		}
		if err := requireDiskType(ctx, device); err != nil {
			return "", err
		}
		protected, err := samePhysicalDiskAsRoot(ctx, device)
		if err != nil {
			return "", err
		}
		if protected {
			return "", errors.New("refusing to delete partitions on the system disk")
		}
		if err := prepareDestructiveChange(ctx, device); err != nil {
			return "", err
		}
		if err := deleteAllPartitions(ctx, device); err != nil {
			return "", err
		}
		return "all partitions deleted", nil

	case "storage.label.rename":
		targets, err := mountedTargets(ctx, device)
		if err != nil {
			return "", err
		}
		if len(targets) != 0 {
			return "", errors.New("device must be unmounted before renaming")
		}
		protected, err := samePhysicalDiskAsRoot(ctx, device)
		if err != nil {
			return "", err
		}
		if protected {
			return "", errors.New("refusing to rename a filesystem on the system disk")
		}
		label := strings.TrimSpace(request.Label)
		if label == "" {
			return "", errors.New("filesystem label cannot be empty")
		}
		fstypeOutput, err := exec.CommandContext(ctx, "/usr/bin/lsblk", "-ndo", "FSTYPE", device).CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("inspect filesystem type: %s", strings.TrimSpace(string(fstypeOutput)))
		}
		fstype := strings.ToLower(strings.TrimSpace(string(fstypeOutput)))
		if !validFilesystemLabelForType(label, fstype) {
			return "", errors.New("invalid filesystem label for filesystem type")
		}
		var command string
		var args []string
		switch fstype {
		case "ext4":
			command = "/usr/sbin/e2label"
			args = []string{device, label}
		case "xfs":
			command = "/usr/sbin/xfs_admin"
			args = []string{"-L", label, device}
		case "vfat", "fat", "fat32":
			command = "/usr/sbin/fatlabel"
			args = []string{device, label}
		default:
			return "", errors.New("renaming is supported for ext4, xfs and vfat filesystems")
		}
		if _, err := os.Stat(command); err != nil {
			return "", fmt.Errorf("filesystem label tool is unavailable: %s", command)
		}
		if output, err := exec.CommandContext(ctx, command, args...).CombinedOutput(); err != nil {
			return "", fmt.Errorf("rename filesystem: %s", strings.TrimSpace(string(output)))
		}
		_ = exec.CommandContext(ctx, "/usr/bin/udevadm", "settle").Run()
		labelOutput, err := exec.CommandContext(ctx, "/usr/bin/lsblk", "-ndo", "LABEL", device).CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("verify filesystem label: %s", strings.TrimSpace(string(labelOutput)))
		}
		if strings.TrimSpace(string(labelOutput)) != label {
			return "", fmt.Errorf("filesystem label verification failed")
		}
		return "filesystem renamed", nil

	case "storage.format":
		if request.Confirm != "FORMAT "+device {
			return "", errors.New("format confirmation does not match device")
		}
		protected, err := samePhysicalDiskAsRoot(ctx, device)
		if err != nil {
			return "", err
		}
		if protected {
			return "", errors.New("refusing to format a device on the system disk")
		}
		if err := prepareDestructiveChange(ctx, device); err != nil {
			return "", err
		}
		label := strings.TrimSpace(request.Label)
		if !validFilesystemLabel(label) {
			return "", errors.New("invalid filesystem label")
		}

		var command string
		var args []string
		switch strings.ToLower(strings.TrimSpace(request.Filesystem)) {
		case "ext4":
			command = "/usr/sbin/mkfs.ext4"
			args = []string{"-F", "-m", "0", "-O", "quota,project", "-E", "quotatype=usrquota:prjquota"}
			if label != "" {
				args = append(args, "-L", label)
			}
		case "xfs":
			command = "/usr/sbin/mkfs.xfs"
			args = []string{"-f"}
			if label != "" {
				args = append(args, "-L", label)
			}
		case "vfat":
			command = "/usr/sbin/mkfs.vfat"
			if label != "" {
				args = append(args, "-n", label)
			}
		default:
			return "", errors.New("unsupported filesystem; use ext4, xfs or vfat")
		}
		if _, err := os.Stat(command); err != nil {
			return "", fmt.Errorf("filesystem tool is unavailable: %s", command)
		}
		args = append(args, device)
		if output, err := exec.CommandContext(ctx, command, args...).CombinedOutput(); err != nil {
			return "", fmt.Errorf("format device: %s", strings.TrimSpace(string(output)))
		}
		_ = exec.CommandContext(ctx, "/usr/bin/udevadm", "settle").Run()
		actualOutput, err := exec.CommandContext(ctx, "/usr/bin/lsblk", "-ndo", "FSTYPE", device).CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("verify filesystem: %s", strings.TrimSpace(string(actualOutput)))
		}
		actual := strings.ToLower(strings.TrimSpace(string(actualOutput)))
		expected := strings.ToLower(strings.TrimSpace(request.Filesystem))
		if expected == "vfat" && (actual == "fat" || actual == "fat32") {
			actual = "vfat"
		}
		if actual != expected {
			return "", fmt.Errorf("filesystem verification failed: expected %s, got %s", expected, actual)
		}
		if label != "" {
			labelOutput, err := exec.CommandContext(ctx, "/usr/bin/lsblk", "-ndo", "LABEL", device).CombinedOutput()
			if err != nil {
				return "", fmt.Errorf("verify filesystem label: %s", strings.TrimSpace(string(labelOutput)))
			}
			if strings.TrimSpace(string(labelOutput)) != label {
				return "", errors.New("filesystem label verification failed")
			}
		}
		return "device formatted", nil
	default:
		return "", errors.New("unsupported storage operation")
	}
}

func filesystemMountDiagnostic(ctx context.Context, device, filesystem string) string {
	command, args := filesystemDiagnosticCommand(device, filesystem)
	if command == "" {
		return ""
	}
	if _, err := os.Stat(command); err != nil {
		return "diagnostic tool is unavailable"
	}
	output, err := exec.CommandContext(ctx, command, args...).CombinedOutput()
	message := strings.TrimSpace(string(output))
	if message == "" && err != nil {
		message = err.Error()
	}
	if message == "" {
		return "filesystem check returned no details"
	}
	return compactFilesystemDiagnostic(message)
}

func filesystemDiagnosticCommand(device, filesystem string) (string, []string) {
	switch strings.ToLower(strings.TrimSpace(filesystem)) {
	case "ext2", "ext3", "ext4":
		return "/usr/sbin/e2fsck", []string{"-n", device}
	case "xfs":
		return "/usr/sbin/xfs_repair", []string{"-n", device}
	case "vfat", "fat", "fat32":
		return "/usr/sbin/fsck.fat", []string{"-n", "-v", device}
	default:
		return "", nil
	}
}

func compactFilesystemDiagnostic(message string) string {
	message = strings.Join(strings.Fields(message), " ")
	const maxDiagnosticBytes = 3500
	if len(message) > maxDiagnosticBytes {
		return message[:maxDiagnosticBytes] + "…"
	}
	return message
}

func requireDiskType(ctx context.Context, device string) error {
	output, err := exec.CommandContext(ctx, "/usr/bin/lsblk", "-ndo", "TYPE", device).CombinedOutput()
	if err != nil {
		return fmt.Errorf("inspect disk type: %s", strings.TrimSpace(string(output)))
	}
	if strings.TrimSpace(string(output)) != "disk" {
		return errors.New("partition creation is allowed only on physical disks")
	}
	return nil
}

func requirePartitionType(ctx context.Context, device string) error {
	output, err := exec.CommandContext(ctx, "/usr/bin/lsblk", "-ndo", "TYPE", device).CombinedOutput()
	if err != nil {
		return fmt.Errorf("inspect partition type: %s", strings.TrimSpace(string(output)))
	}
	if strings.TrimSpace(string(output)) != "part" {
		return errors.New("partition deletion is allowed only for partitions")
	}
	return nil
}

func diskHasMountedDescendants(ctx context.Context, device string) (bool, error) {
	output, err := exec.CommandContext(ctx, "/usr/bin/lsblk", "-nrpo", "MOUNTPOINTS", device).CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("inspect disk mounts: %s", strings.TrimSpace(string(output)))
	}
	for _, line := range strings.Split(string(output), "\n") {
		if strings.TrimSpace(line) != "" {
			return true, nil
		}
	}
	return false, nil
}

func remainingDiskBytes(ctx context.Context, disk string) (uint64, error) {
	output, err := exec.CommandContext(ctx, "/usr/bin/lsblk", "-bnro", "TYPE,SIZE", disk).CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("inspect disk capacity: %s", strings.TrimSpace(string(output)))
	}
	var total uint64
	var allocated uint64
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		if fields[0] == "disk" && total == 0 {
			total = value
		}
		if fields[0] == "part" {
			allocated += value
		}
	}
	if total == 0 {
		return 0, errors.New("cannot determine disk capacity")
	}
	if allocated >= total {
		return 0, nil
	}
	return total - allocated, nil
}

func createPartition(ctx context.Context, disk string, sizeMiB uint64) error {
	before, err := partitionNames(ctx, disk)
	if err != nil {
		return err
	}

	pttypeOutput, err := exec.CommandContext(ctx, "/usr/bin/lsblk", "-ndo", "PTTYPE", disk).CombinedOutput()
	if err != nil {
		return fmt.Errorf("inspect partition table: %s", strings.TrimSpace(string(pttypeOutput)))
	}
	pttype := strings.TrimSpace(string(pttypeOutput))

	var args []string
	if pttype == "" {
		args = []string{"--label", "gpt", disk}
	} else {
		args = []string{"--append", disk}
	}

	line := ",\n"
	if sizeMiB > 0 {
		line = fmt.Sprintf(",%dMiB\n", sizeMiB)
	}
	command := exec.CommandContext(ctx, "/usr/sbin/sfdisk", args...)
	command.Stdin = strings.NewReader(line)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("create partition: %s", strings.TrimSpace(string(output)))
	}

	reloadOutput, err := exec.CommandContext(ctx, "/usr/sbin/blockdev", "--rereadpt", disk).CombinedOutput()
	if err != nil {
		return fmt.Errorf(
			"partition was written but kernel could not reload the table: %s",
			strings.TrimSpace(string(reloadOutput)),
		)
	}
	_ = exec.CommandContext(ctx, "/usr/bin/udevadm", "settle").Run()

	after, err := partitionNames(ctx, disk)
	if err != nil {
		return err
	}
	if len(after) <= len(before) {
		return errors.New("partition table was changed but the new partition is not visible to the kernel")
	}
	return nil
}

func prepareDestructiveChange(ctx context.Context, device string) error {
	activeSwaps, err := activeSwapDevices()
	if err != nil {
		return err
	}

	output, err := exec.CommandContext(ctx, "/usr/bin/lsblk", "-nrpo", "NAME,FSTYPE", device).CombinedOutput()
	if err != nil {
		return fmt.Errorf("inspect device usage: %s", strings.TrimSpace(string(output)))
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		fields := strings.Fields(lines[i])
		if len(fields) == 0 {
			continue
		}
		name := fields[0]
		fstype := ""
		if len(fields) > 1 {
			fstype = fields[1]
		}
		if fstype == "swap" {
			if !devicePathInSet(name, activeSwaps) {
				continue
			}
			if out, err := exec.CommandContext(ctx, "/usr/sbin/swapoff", "--", name).CombinedOutput(); err != nil {
				message := strings.TrimSpace(string(out))
				if message == "" {
					message = err.Error()
				}
				return fmt.Errorf("disable active swap %s: %s", name, message)
			}
			continue
		}
		targets, err := mountedTargets(ctx, name)
		if err != nil {
			return err
		}
		if len(targets) != 0 {
			for _, target := range targets {
				if target == "/" {
					return errors.New("refusing to unmount the root filesystem")
				}
			}
			if err := unmountTargets(ctx, name, targets); err != nil {
				return err
			}
		}
	}
	if err := deactivateLVMOnDisk(ctx, device); err != nil {
		return err
	}
	return nil
}

func activeSwapDevices() (map[string]bool, error) {
	data, err := os.ReadFile("/proc/swaps")
	if err != nil {
		return nil, fmt.Errorf("inspect active swap devices: %w", err)
	}
	result := map[string]bool{}
	lines := strings.Split(string(data), "\n")
	for index, line := range lines {
		if index == 0 {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		path := fields[0]
		result[path] = true
		if resolved, err := filepath.EvalSymlinks(path); err == nil {
			result[resolved] = true
		}
	}
	return result, nil
}

func devicePathInSet(device string, paths map[string]bool) bool {
	if paths[device] {
		return true
	}
	resolved, err := filepath.EvalSymlinks(device)
	if err != nil {
		return false
	}
	return paths[resolved]
}

func deactivateLVMOnDisk(ctx context.Context, device string) error {
	if _, err := os.Stat("/usr/sbin/pvs"); err != nil {
		return nil
	}
	if _, err := os.Stat("/usr/sbin/vgchange"); err != nil {
		return nil
	}

	targetDisk, err := topPhysicalDisk(ctx, device)
	if err != nil {
		return err
	}

	output, err := exec.CommandContext(
		ctx,
		"/usr/sbin/pvs",
		"--noheadings",
		"--separator", "|",
		"-o", "pv_name,vg_name",
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("inspect LVM physical volumes: %s", strings.TrimSpace(string(output)))
	}

	type pvVG struct {
		pv string
		vg string
	}
	var mappings []pvVG
	targetVGs := map[string]bool{}
	for _, line := range strings.Split(string(output), "\n") {
		parts := strings.Split(line, "|")
		if len(parts) < 2 {
			continue
		}
		pv := strings.TrimSpace(parts[0])
		vg := strings.TrimSpace(parts[1])
		if pv == "" || vg == "" || !strings.HasPrefix(pv, "/dev/") {
			continue
		}
		mappings = append(mappings, pvVG{pv: pv, vg: vg})
		disk, err := topPhysicalDisk(ctx, pv)
		if err != nil {
			continue
		}
		if disk == targetDisk {
			targetVGs[vg] = true
		}
	}

	for vg := range targetVGs {
		for _, mapping := range mappings {
			if mapping.vg != vg {
				continue
			}
			disk, err := topPhysicalDisk(ctx, mapping.pv)
			if err != nil {
				return fmt.Errorf("resolve LVM physical volume %s: %w", mapping.pv, err)
			}
			if disk != targetDisk {
				return fmt.Errorf("refusing to deactivate LVM volume group %s because it also uses another physical disk", vg)
			}
		}
		out, err := exec.CommandContext(ctx, "/usr/sbin/vgchange", "-an", "--", vg).CombinedOutput()
		if err != nil {
			return fmt.Errorf("deactivate LVM volume group %s: %s", vg, strings.TrimSpace(string(out)))
		}
		_ = exec.CommandContext(ctx, "/usr/bin/udevadm", "settle").Run()
		attrOutput, err := exec.CommandContext(
			ctx,
			"/usr/sbin/lvs",
			"--noheadings",
			"-o", "lv_attr",
			"--select", "vg_name="+vg,
		).CombinedOutput()
		if err != nil {
			return fmt.Errorf("verify LVM volume group %s: %s", vg, strings.TrimSpace(string(attrOutput)))
		}
		for _, line := range strings.Split(string(attrOutput), "\n") {
			attr := strings.TrimSpace(line)
			if len(attr) > 4 && attr[4] == 'a' {
				return fmt.Errorf("LVM volume group %s is still active after deactivation", vg)
			}
		}
	}
	return nil
}

func deleteAllPartitions(ctx context.Context, disk string) error {
	before, err := partitionNames(ctx, disk)
	if err != nil {
		return err
	}

	output, err := exec.CommandContext(ctx, "/usr/sbin/sfdisk", "--delete", disk).CombinedOutput()
	if err != nil {
		return fmt.Errorf("delete all partitions: %s", strings.TrimSpace(string(output)))
	}

	if err := rereadPartitionTable(ctx, disk); err != nil {
		return err
	}

	after, err := partitionNames(ctx, disk)
	if err != nil {
		return err
	}
	if len(after) != 0 {
		return fmt.Errorf("kernel still reports partitions after deletion: %s", strings.Join(after, ", "))
	}
	if len(before) == 0 {
		return errors.New("disk has no partitions to delete")
	}
	return nil
}

func deletePartition(ctx context.Context, partition string) error {
	parentOutput, err := exec.CommandContext(ctx, "/usr/bin/lsblk", "-ndo", "PKNAME", partition).CombinedOutput()
	if err != nil {
		return fmt.Errorf("resolve parent disk: %s", strings.TrimSpace(string(parentOutput)))
	}
	parent := strings.TrimSpace(string(parentOutput))
	if parent == "" {
		return errors.New("cannot resolve parent disk")
	}
	parentDevice := filepath.Join("/dev", parent)
	if err := requireDiskType(ctx, parentDevice); err != nil {
		return err
	}

	partNumberData, err := os.ReadFile(filepath.Join("/sys/class/block", filepath.Base(partition), "partition"))
	if err != nil {
		return fmt.Errorf("resolve partition number: %w", err)
	}
	partNumber := strings.TrimSpace(string(partNumberData))
	if partNumber == "" {
		return errors.New("cannot resolve partition number")
	}

	output, err := exec.CommandContext(ctx, "/usr/sbin/sfdisk", "--delete", parentDevice, partNumber).CombinedOutput()
	if err != nil {
		return fmt.Errorf("delete partition: %s", strings.TrimSpace(string(output)))
	}
	if err := rereadPartitionTable(ctx, parentDevice); err != nil {
		return err
	}
	if _, err := os.Stat(partition); err == nil {
		return fmt.Errorf("kernel still reports partition %s after deletion", partition)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("verify deleted partition: %w", err)
	}
	return nil
}

func rereadPartitionTable(ctx context.Context, disk string) error {
	output, err := exec.CommandContext(ctx, "/usr/sbin/blockdev", "--rereadpt", disk).CombinedOutput()
	if err != nil {
		// partx can remove stale kernel partition mappings after the on-disk table
		// has changed. It still refuses entries that are genuinely busy.
		_, _ = exec.CommandContext(ctx, "/usr/bin/partx", "--delete", disk).CombinedOutput()
		output, err = exec.CommandContext(ctx, "/usr/sbin/blockdev", "--rereadpt", disk).CombinedOutput()
	}
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("partition table changed on disk but kernel could not reload it; close/deactivate volumes that still use this disk: %s", message)
	}
	_ = exec.CommandContext(ctx, "/usr/bin/udevadm", "settle").Run()
	return nil
}

func partitionNames(ctx context.Context, disk string) ([]string, error) {
	output, err := exec.CommandContext(ctx, "/usr/bin/lsblk", "-nrpo", "NAME,TYPE", disk).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("inspect disk partitions: %s", strings.TrimSpace(string(output)))
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == "part" {
			names = append(names, fields[0])
		}
	}
	return names, nil
}

func validateBlockDevice(value string) (string, error) {
	device := filepath.Clean(strings.TrimSpace(value))
	if device == "." || !strings.HasPrefix(device, "/dev/") {
		return "", errors.New("invalid block device path")
	}
	info, err := os.Stat(device)
	if err != nil {
		return "", fmt.Errorf("stat block device: %w", err)
	}
	if info.Mode()&os.ModeDevice == 0 || info.Mode()&os.ModeCharDevice != 0 {
		return "", errors.New("requested path is not a block device")
	}
	return device, nil
}

func unmountTargets(ctx context.Context, device string, targets []string) error {
	for i := len(targets) - 1; i >= 0; i-- {
		target := targets[i]
		if out, err := exec.CommandContext(ctx, "/usr/bin/umount", "--", target).CombinedOutput(); err != nil {
			message := strings.TrimSpace(string(out))
			if message == "" {
				message = err.Error()
			}
			return fmt.Errorf("unmount %s from %s: %s", device, target, message)
		}
	}
	remaining, err := mountedTargets(ctx, device)
	if err != nil {
		return fmt.Errorf("verify unmount %s: %w", device, err)
	}
	if len(remaining) != 0 {
		return fmt.Errorf("unmount completed but %s is still mounted at: %s", device, strings.Join(remaining, ", "))
	}
	return nil
}

func mountedTargets(ctx context.Context, device string) ([]string, error) {
	output, err := exec.CommandContext(ctx, "/usr/bin/findmnt", "-rn", "-S", device, "-o", "TARGET").CombinedOutput()
	if err != nil {
		// findmnt exits non-zero when the source has no mounts.
		if len(strings.TrimSpace(string(output))) == 0 {
			return []string{}, nil
		}
		return nil, fmt.Errorf("inspect mounts: %s", strings.TrimSpace(string(output)))
	}
	var targets []string
	for _, line := range strings.Split(string(output), "\n") {
		if target := strings.TrimSpace(line); target != "" {
			targets = append(targets, target)
		}
	}
	return targets, nil
}

func samePhysicalDiskAsRoot(ctx context.Context, device string) (bool, error) {
	rootOutput, err := exec.CommandContext(ctx, "/usr/bin/findmnt", "-rn", "-o", "SOURCE", "/").CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("resolve root filesystem: %s", strings.TrimSpace(string(rootOutput)))
	}
	rootSource := strings.TrimSpace(string(rootOutput))
	if index := strings.Index(rootSource, "["); index >= 0 {
		rootSource = rootSource[:index]
	}
	if !strings.HasPrefix(rootSource, "/dev/") {
		return false, errors.New("cannot safely resolve the system disk")
	}
	rootDisk, err := topPhysicalDisk(ctx, rootSource)
	if err != nil {
		return false, err
	}
	targetDisk, err := topPhysicalDisk(ctx, device)
	if err != nil {
		return false, err
	}
	return rootDisk == targetDisk, nil
}

func topPhysicalDisk(ctx context.Context, device string) (string, error) {
	current := device
	for i := 0; i < 16; i++ {
		output, err := exec.CommandContext(ctx, "/usr/bin/lsblk", "-ndo", "PKNAME", current).CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("resolve parent disk: %s", strings.TrimSpace(string(output)))
		}
		parent := strings.TrimSpace(string(output))
		if parent == "" {
			return filepath.Base(current), nil
		}
		current = filepath.Join("/dev", parent)
	}
	return "", errors.New("block device parent chain is too deep")
}

func validFilesystemLabelForType(value, filesystem string) bool {
	limit := 32
	switch filesystem {
	case "ext4":
		limit = 16
	case "xfs":
		limit = 12
	case "vfat", "fat", "fat32":
		limit = 11
	}
	if len(value) == 0 || len(value) > limit {
		return false
	}
	return validFilesystemLabel(value)
}

func validFilesystemLabel(value string) bool {
	if len(value) > 32 {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == ' ' || r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}
