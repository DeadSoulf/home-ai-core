package systeminfo

import (
	"bufio"
	"io"
	"os"
	"strconv"
	"strings"
)

func memoryInfo() MemoryInfo {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return MemoryInfo{}
	}
	defer file.Close()

	return memoryInfoFromReader(file)
}

func memoryInfoFromReader(reader io.Reader) MemoryInfo {
	var info MemoryInfo

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}

		kib, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}

		switch fields[0] {
		case "MemTotal:":
			info.TotalBytes = kib * 1024
		case "MemAvailable:":
			info.AvailableBytes = kib * 1024
		}
	}

	return info
}
