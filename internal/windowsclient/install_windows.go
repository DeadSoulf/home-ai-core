//go:build windows

package windowsclient

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"
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

func InstallUserClientWithHandoff(source string, timeout time.Duration) (UserClientInstallResult, error) {
	destination, err := UserClientInstallPath()
	if err != nil {
		return UserClientInstallResult{}, err
	}
	if sameCleanPath(source, destination) {
		return UserClientInstallResult{Path: destination}, nil
	}
	if same, err := sameRegularFileContent(source, destination); err == nil && same {
		return UserClientInstallResult{Path: destination}, nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return UserClientInstallResult{}, fmt.Errorf("compare installed Windows client: %w", err)
	}

	if err := installExecutable(source, destination); err == nil {
		return UserClientInstallResult{Path: destination, Changed: true}, nil
	} else if !isExecutableReplaceBlocked(err) {
		return UserClientInstallResult{}, err
	}

	requested, err := RequestUserAgentExit()
	if err != nil {
		return UserClientInstallResult{}, err
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		if err := installExecutable(source, destination); err == nil {
			return UserClientInstallResult{
				Path:               destination,
				Changed:            true,
				AgentExitRequested: requested,
			}, nil
		} else {
			lastErr = err
			if !isExecutableReplaceBlocked(err) {
				return UserClientInstallResult{}, err
			}
		}
		if time.Now().After(deadline) {
			if requested {
				return UserClientInstallResult{}, fmt.Errorf("replace installed Windows client after agent exit: %w", lastErr)
			}
			return UserClientInstallResult{}, fmt.Errorf("replace installed Windows client after update handoff: %w", lastErr)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func isExecutableReplaceBlocked(err error) bool {
	return errors.Is(err, windows.ERROR_ACCESS_DENIED) ||
		errors.Is(err, windows.ERROR_SHARING_VIOLATION) ||
		errors.Is(err, windows.ERROR_LOCK_VIOLATION)
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
