//go:build windows

package windowsclient

import (
	"fmt"
	"os"
	"path/filepath"
)

const InstalledWindowsClientName = "home-ai-windows-client.exe"

func UserClientInstallPath() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("locate LocalAppData: %w", err)
	}
	return filepath.Join(base, "HomeAI", "bin", InstalledWindowsClientName), nil
}

func InstallUserClient(source string) (string, error) {
	destination, err := UserClientInstallPath()
	if err != nil {
		return "", err
	}
	if err := installExecutable(source, destination); err != nil {
		return "", err
	}
	return destination, nil
}

func UserClientInstallStatus() (string, bool, error) {
	path, err := UserClientInstallPath()
	if err != nil {
		return "", false, err
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return path, false, nil
	}
	if err != nil {
		return path, false, fmt.Errorf("inspect installed Windows client: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return path, false, fmt.Errorf("installed Windows client path is not a real regular file")
	}
	return path, true, nil
}
