//go:build !windows

package windowsclient

func StartVerifiedClientInstall(executable string) error {
	return ErrUserClientInstallUnsupported
}
