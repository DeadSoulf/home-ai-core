package main

import "strings"

// --updated lets the new GUI show a clear success state after relaunch.
func windowsSettingsReopenArgs(configPath, settingsPath string) []string {
	args := []string{"settings", "--updated"}
	if strings.TrimSpace(configPath) != "" {
		args = append(args, "--config", configPath)
	}
	if strings.TrimSpace(settingsPath) != "" {
		args = append(args, "--settings", settingsPath)
	}
	return args
}
