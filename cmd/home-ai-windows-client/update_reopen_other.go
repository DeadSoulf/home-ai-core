//go:build !windows

package main

import (
	"errors"
)

func reopenWindowsClientSettings(executable, configPath, settingsPath string) error {
	return errors.New("reopening Windows settings is only supported on Windows")
}

func notifyWindowsClientUpdateFailure(err error) {}
