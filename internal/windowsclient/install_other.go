//go:build !windows

package windowsclient

import "time"

func UserClientInstallPath() (string, error) {
	return "", ErrUserClientInstallUnsupported
}

func InstallUserClient(source string) (string, error) {
	return "", ErrUserClientInstallUnsupported
}

func UserClientInstallStatus() (string, bool, error) {
	return "", false, ErrUserClientInstallUnsupported
}

func InstallUserClientWithHandoff(source string, timeout time.Duration) (UserClientInstallResult, error) {
	return UserClientInstallResult{}, ErrUserClientInstallUnsupported
}
