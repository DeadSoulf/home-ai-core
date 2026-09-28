package systeminfo

import (
	"bufio"
	"os"
	"runtime"
	"strconv"
	"strings"
)

type Info struct {
	NodeID           string `json:"node_id"`
	Hostname         string `json:"hostname"`
	OS               string `json:"os"`
	Architecture     string `json:"architecture"`
	CPUCount         int    `json:"cpu_count"`
	MemoryTotalBytes uint64 `json:"memory_total_bytes,omitempty"`
	UptimeSeconds    uint64 `json:"uptime_seconds,omitempty"`
}

func Collect(nodeID string) Info {
	hostname, _ := os.Hostname()

	return Info{
		NodeID:           nodeID,
		Hostname:         hostname,
		OS:               operatingSystem(),
		Architecture:     runtime.GOARCH,
		CPUCount:         runtime.NumCPU(),
		MemoryTotalBytes: memoryTotal(),
		UptimeSeconds:    uptime(),
	}
}

func operatingSystem() string {
	file, err := os.Open("/etc/os-release")
	if err != nil {
		return runtime.GOOS
	}
	defer file.Close()

	values := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		values[key] = strings.Trim(value, "\"")
	}

	if value := values["PRETTY_NAME"]; value != "" {
		return value
	}
	if value := values["NAME"]; value != "" {
		return value
	}
	return runtime.GOOS
}

func memoryTotal() uint64 {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 || fields[0] != "MemTotal:" {
			continue
		}
		kib, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return 0
		}
		return kib * 1024
	}
	return 0
}

func uptime() uint64 {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0
	}
	seconds, err := strconv.ParseFloat(fields[0], 64)
	if err != nil || seconds < 0 {
		return 0
	}
	return uint64(seconds)
}
