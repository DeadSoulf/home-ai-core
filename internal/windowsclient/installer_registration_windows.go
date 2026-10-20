//go:build windows

package windowsclient

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const windowsClientUninstallRegistryKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\HomeAIWindowsClient`

var windowsClientInstallerVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.]+)?$`)

// RefreshInstalledClientVersion updates the version displayed in Windows'
// Installed apps when an already registered NSIS installation is upgraded
// through the built-in Go updater. It never invents an Apps entry for a
// standalone portable install.
func RefreshInstalledClientVersion(value string) error {
	value = strings.TrimSpace(value)
	if !windowsClientInstallerVersion.MatchString(value) {
		return nil // Preview/developer builds must not label Apps with "dev".
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, windowsClientUninstallRegistryKey,
		registry.QUERY_VALUE|registry.SET_VALUE)
	if err == registry.ErrNotExist || err == windows.ERROR_FILE_NOT_FOUND {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open HOME AI installation metadata: %w", err)
	}
	defer key.Close()
	path, _, err := key.GetStringValue("InstallLocation")
	if err != nil {
		return fmt.Errorf("read HOME AI installed location: %w", err)
	}
	expected, err := UserClientInstallPath()
	if err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Clean(path), filepath.Clean(expected)) {
		return fmt.Errorf("HOME AI installation metadata points to a different directory")
	}
	if err := key.SetStringValue("DisplayVersion", value); err != nil {
		return fmt.Errorf("update installed HOME AI version: %w", err)
	}
	return nil
}
