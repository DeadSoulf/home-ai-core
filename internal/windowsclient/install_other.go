//go:build !windows

package windowsclient

func UserClientInstallPath() (string, error) {
	return "", ErrUserClientInstallUnsupported
}

func InstallUserClient(source string) (string, error) {
	return "", ErrUserClientInstallUnsupported
}

func UserClientInstallStatus() (string, bool, error) {
	return "", false, ErrUserClientInstallUnsupported
}
