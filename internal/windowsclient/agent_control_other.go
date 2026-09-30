//go:build !windows

package windowsclient

func RequestUserAgentExit() (bool, error) {
	return false, ErrUserAutostartUnsupported
}

func StartUserAgent(executable, configPath string) error {
	return ErrUserAutostartUnsupported
}


func UserAgentRunning() bool {
	return false
}

func SignalUserAgentSyncNow() (bool, error) {
	return false, ErrCredentialStoreUnsupported
}
