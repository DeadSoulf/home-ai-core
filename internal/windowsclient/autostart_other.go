//go:build !windows

package windowsclient

func InstallUserAgentAutostart(executable, configPath string) error {
	return ErrUserAutostartUnsupported
}

func UserAgentAutostartStatus() (UserAutostartInfo, error) {
	return UserAutostartInfo{}, ErrUserAutostartUnsupported
}

func RemoveUserAgentAutostart() error {
	return ErrUserAutostartUnsupported
}
