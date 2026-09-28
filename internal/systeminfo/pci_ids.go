package systeminfo

import (
	"bufio"
	"os"
	"strings"
)

type pciIDDatabase struct {
	vendors map[string]string
	devices map[string]map[string]string
}

func loadPCIIDs(paths []string) pciIDDatabase {
	db := pciIDDatabase{
		vendors: map[string]string{},
		devices: map[string]map[string]string{},
	}
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		parsePCIIDs(file, &db)
		_ = file.Close()
		if len(db.vendors) > 0 {
			break
		}
	}
	return db
}

func parsePCIIDs(file *os.File, db *pciIDDatabase) {
	scanner := bufio.NewScanner(file)
	currentVendor := ""
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "\t\t") {
			continue
		}
		if strings.HasPrefix(line, "\t") {
			if currentVendor == "" {
				continue
			}
			fields := strings.Fields(strings.TrimSpace(line))
			if len(fields) < 2 || len(fields[0]) != 4 {
				continue
			}
			deviceID := strings.ToLower(fields[0])
			name := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), fields[0]))
			if db.devices[currentVendor] == nil {
				db.devices[currentVendor] = map[string]string{}
			}
			db.devices[currentVendor][deviceID] = name
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 || len(fields[0]) != 4 {
			currentVendor = ""
			continue
		}
		currentVendor = strings.ToLower(fields[0])
		db.vendors[currentVendor] = strings.TrimSpace(strings.TrimPrefix(line, fields[0]))
	}
}

func (db pciIDDatabase) lookup(vendorID, deviceID string) (string, string) {
	vendorID = normalizePCIHex(vendorID)
	deviceID = normalizePCIHex(deviceID)
	vendor := db.vendors[vendorID]
	model := ""
	if devices := db.devices[vendorID]; devices != nil {
		model = devices[deviceID]
	}
	return vendor, model
}
