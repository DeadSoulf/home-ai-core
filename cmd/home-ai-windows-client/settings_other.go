//go:build !windows

package main

import "errors"

func runDefaultCommand() error {
	printUsage()
	return errors.New("command is required")
}

func runSettingsUI(args []string) error {
	return errors.New("Windows settings UI is only available on Windows")
}

func startSettingsProcess(configPath string) error {
	return errors.New("Windows settings UI is only available on Windows")
}
