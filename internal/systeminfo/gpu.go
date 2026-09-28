package systeminfo

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func gpus(drmRoot string) []GPU {
	cards, err := filepath.Glob(filepath.Join(drmRoot, "card[0-9]*"))
	if err != nil {
		return []GPU{}
	}

	result := make([]GPU, 0, len(cards))
	for _, cardPath := range cards {
		info, ok := gpuFromCard(cardPath)
		if !ok {
			continue
		}
		result = append(result, info)
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].PCIAddress != result[j].PCIAddress {
			return result[i].PCIAddress < result[j].PCIAddress
		}
		return result[i].Card < result[j].Card
	})
	return result
}

func gpuFromCard(cardPath string) (GPU, bool) {
	devicePath := filepath.Join(cardPath, "device")
	vendorID := strings.ToLower(readTrimmed(filepath.Join(devicePath, "vendor")))
	deviceID := strings.ToLower(readTrimmed(filepath.Join(devicePath, "device")))

	if vendorID == "" && deviceID == "" {
		return GPU{}, false
	}

	uevent := readUEvent(filepath.Join(devicePath, "uevent"))
	return GPU{
		Card:       filepath.Base(cardPath),
		Vendor:     gpuVendorName(vendorID),
		VendorID:   vendorID,
		DeviceID:   deviceID,
		Driver:     uevent["DRIVER"],
		PCIAddress: uevent["PCI_SLOT_NAME"],
	}, true
}

func readUEvent(path string) map[string]string {
	values := map[string]string{}

	file, err := os.Open(path)
	if err != nil {
		return values
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), "=")
		if !ok {
			continue
		}
		values[key] = value
	}
	return values
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
	case "0x1234":
		return "QEMU"
	default:
		return ""
	}
}
