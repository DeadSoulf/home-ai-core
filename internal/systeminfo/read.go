package systeminfo

import (
	"os"
	"strconv"
	"strings"
)

func readTrimmed(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func readUint(path string) uint64 {
	value := readTrimmed(path)
	if value == "" {
		return 0
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func readBool01(path string) bool {
	return readTrimmed(path) == "1"
}
