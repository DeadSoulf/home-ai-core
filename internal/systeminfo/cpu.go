package systeminfo

import (
	"bufio"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func cpuInfo() CPUInfo {
	info := CPUInfo{LogicalCPUs: runtime.NumCPU()}

	file, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return info
	}
	defer file.Close()

	info.Model = cpuModelFromReader(file)
	info.UsagePercent = cpuUsagePercent()
	return info
}

func cpuUsagePercent() float64 {
	idle1, total1, ok := readCPUTimes()
	if !ok {
		return 0
	}
	time.Sleep(100 * time.Millisecond)
	idle2, total2, ok := readCPUTimes()
	if !ok || total2 <= total1 {
		return 0
	}
	totalDelta := total2 - total1
	idleDelta := idle2 - idle1
	usage := float64(totalDelta-idleDelta) * 100 / float64(totalDelta)
	if usage < 0 {
		return 0
	}
	if usage > 100 {
		return 100
	}
	return usage
}

func readCPUTimes() (idle, total uint64, ok bool) {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0, false
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return 0, 0, false
	}
	fields := strings.Fields(scanner.Text())
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0, 0, false
	}
	values := make([]uint64, 0, len(fields)-1)
	for _, field := range fields[1:] {
		value, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			return 0, 0, false
		}
		values = append(values, value)
		total += value
	}
	idle = values[3]
	if len(values) > 4 {
		idle += values[4]
	}
	return idle, total, true
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
