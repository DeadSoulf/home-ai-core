package systeminfo

import (
	"bufio"
	"os"
	"runtime"
	"strings"
)

func Collect(nodeID string) Info {
	hostname, _ := os.Hostname()

	return Info{
		NodeID:            nodeID,
		Hostname:          hostname,
		OS:                operatingSystem(),
		Kernel:            readTrimmed("/proc/sys/kernel/osrelease"),
		Architecture:      runtime.GOARCH,
		CPU:               cpuInfo(),
		Memory:            memoryInfo(),
		UptimeSeconds:     uptime(),
		BlockDevices:      blockDevices("/sys/block"),
		BlockTree:         lsblkTree(),
		NetworkInterfaces: networkInterfaces("/sys/class/net"),
		GPUs: gpus("/sys/bus/pci/devices", []string{
			"/usr/share/misc/pci.ids",
			"/usr/share/hwdata/pci.ids",
		}),
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
