package systeminfo

import (
	"bufio"
	"io"
	"os"
	"runtime"
	"strings"
)

func cpuInfo() CPUInfo {
	info := CPUInfo{LogicalCPUs: runtime.NumCPU()}

	file, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return info
	}
	defer file.Close()

	info.Model = cpuModelFromReader(file)
	return info
}

func cpuModelFromReader(reader io.Reader) string {
	scanner := bufio.NewScanner(reader)
	fallback := ""

	for scanner.Scan() {
		line := scanner.Text()
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}

		key = strings.TrimSpace(strings.ToLower(key))
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}

		switch key {
		case "model name":
			return value
		case "hardware":
			if fallback == "" {
				fallback = value
			}
		case "processor":
			if fallback == "" && !allDigits(value) {
				fallback = value
			}
		}
	}

	return fallback
}

func allDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
