//go:build !windows

package windowsclient

import "errors"

var ErrWindowsClientUpdateUnsupported = errors.New("Windows client self-update is unavailable on this platform")

func UserClientUpdateDir() (string, error) {
	return "", ErrWindowsClientUpdateUnsupported
}

func LaunchUserClientUpdate(update DownloadedWindowsClientUpdate, restartAgentConfig string) error {
	return ErrWindowsClientUpdateUnsupported
}

func ApplyUserClientUpdate(update DownloadedWindowsClientUpdate, waitPID int, restartAgentConfig string) error {
	return ErrWindowsClientUpdateUnsupported
}
