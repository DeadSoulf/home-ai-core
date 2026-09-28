package systeminfo

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func gpus(pciRoot string, pciIDPaths []string) []GPU {
	entries, err := os.ReadDir(pciRoot)
	if err != nil {
		return []GPU{}
	}

	ids := loadPCIIDs(pciIDPaths)
	result := make([]GPU, 0)
	for _, entry := range entries {
		if !entry.IsDir() && entry.Type()&os.ModeSymlink == 0 {
			continue
		}
		devicePath := filepath.Join(pciRoot, entry.Name())
		classID := strings.ToLower(readTrimmed(filepath.Join(devicePath, "class")))
		if !strings.HasPrefix(strings.TrimPrefix(classID, "0x"), "03") {
			continue
		}

		info, ok := gpuFromPCIDevice(devicePath, ids)
		if !ok {
			continue
		}
		result = append(result, info)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].PCIAddress < result[j].PCIAddress
	})
	return result
}

func gpuFromPCIDevice(devicePath string, ids pciIDDatabase) (GPU, bool) {
	vendorID := normalizePCIHex(readTrimmed(filepath.Join(devicePath, "vendor")))
	deviceID := normalizePCIHex(readTrimmed(filepath.Join(devicePath, "device")))
	if vendorID == "" || deviceID == "" {
		return GPU{}, false
	}

	classID := strings.ToLower(readTrimmed(filepath.Join(devicePath, "class")))
	card := ""
	if matches, _ := filepath.Glob(filepath.Join(devicePath, "drm", "card[0-9]*")); len(matches) > 0 {
		card = filepath.Base(matches[0])
	}

	driver := ""
	if target, err := filepath.EvalSymlinks(filepath.Join(devicePath, "driver")); err == nil {
		driver = filepath.Base(target)
	}
	if driver == "" {
		driver = readUEvent(filepath.Join(devicePath, "uevent"))["DRIVER"]
	}

	vendor, model := ids.lookup(vendorID, deviceID)
	if vendor == "" {
		vendor = gpuVendorName("0x" + vendorID)
	}
	if model == "" {
		if vendor != "" {
			model = vendor + " device 0x" + deviceID
		} else {
			model = "PCI display device 0x" + vendorID + ":0x" + deviceID
		}
	}

	return GPU{
		Card:       card,
		Vendor:     vendor,
		Model:      model,
		VendorID:   "0x" + vendorID,
		DeviceID:   "0x" + deviceID,
		Class:      classID,
		Driver:     driver,
		Modalias:   readTrimmed(filepath.Join(devicePath, "modalias")),
		PCIAddress: filepath.Base(devicePath),
	}, true
}

func normalizePCIHex(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.TrimPrefix(value, "0x")
	if value == "" {
		return ""
	}
	return value
}

func gpuVendorName(id string) string {
	switch strings.ToLower(id) {
	case "0x10de":
		return "NVIDIA"
	case "0x1002", "0x1022":
		return "AMD"
	case "0x8086":
		return "Intel"
	case "0x1a03":
		return "ASPEED"
	case "0x102b":
		return "Matrox"
	case "0x1234":
		return "QEMU"
	default:
		return ""
	}
}
