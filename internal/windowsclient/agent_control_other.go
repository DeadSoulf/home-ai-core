//go:build !windows

package windowsclient

func RequestUserAgentExit() (bool, error) {
	return false, ErrUserAutostartUnsupported
}

func StartUserAgent(executable, configPath string) error {
	return ErrUserAutostartUnsupported
}
