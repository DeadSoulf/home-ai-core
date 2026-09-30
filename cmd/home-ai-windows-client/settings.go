package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func defaultClientSettingsPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user configuration directory: %w", err)
	}
	return filepath.Join(base, "HomeAI", "windows-client.json"), nil
}
