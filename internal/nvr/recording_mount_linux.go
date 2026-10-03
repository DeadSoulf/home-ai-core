//go:build linux

package nvr

import (
	"os"
	"path/filepath"
	"strings"
)

func (osMountChecker) Mounted(path string) (bool, error) {
	path = filepath.Clean(strings.TrimSpace(path))
	data, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		if filepath.Clean(decodeMountInfoPath(fields[4])) == path {
			return true, nil
		}
	}
	return false, nil
}

func decodeMountInfoPath(value string) string {
	replacer := strings.NewReplacer(
		"\\040", " ",
		"\\011", "\t",
		"\\012", "\n",
		"\\134", "\\",
	)
	return replacer.Replace(value)
}
