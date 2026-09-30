//go:build windows

package windowsclient

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf16"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const windowsRunKey = `Software\Microsoft\Windows\CurrentVersion\Run`

func InstallUserAgentAutostart(executable, configPath string) error {
	command, err := buildUserAgentCommand(executable, configPath)
	if err != nil {
		return err
	}
	if len(utf16.Encode([]rune(command))) >= 260 {
		return errors.New("Windows Run command exceeds the 260-character limit")
	}
	key, _, err := registry.CreateKey(
		registry.CURRENT_USER,
		windowsRunKey,
		registry.SET_VALUE|registry.QUERY_VALUE,
	)
	if err != nil {
		return fmt.Errorf("open Windows user Run key: %w", err)
	}
	defer key.Close()
	if err := key.SetStringValue(WindowsAgentRunValue, command); err != nil {
		return fmt.Errorf("write Windows user Run value: %w", err)
	}
	return nil
}

func UserAgentAutostartStatus() (UserAutostartInfo, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, windowsRunKey, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) || errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		return UserAutostartInfo{}, nil
	}
	if err != nil {
		return UserAutostartInfo{}, fmt.Errorf("open Windows user Run key: %w", err)
	}
	defer key.Close()
	command, _, err := key.GetStringValue(WindowsAgentRunValue)
	if errors.Is(err, registry.ErrNotExist) || errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		return UserAutostartInfo{}, nil
	}
	if err != nil {
		return UserAutostartInfo{}, fmt.Errorf("read Windows user Run value: %w", err)
	}
	command = strings.TrimSpace(command)
	if command == "" {
		return UserAutostartInfo{}, errors.New("Windows user Run value is empty")
	}
	executable, configPath, err := ParseUserAgentCommand(command)
	if err != nil {
		return UserAutostartInfo{}, fmt.Errorf("parse Windows user Run value: %w", err)
	}
	return UserAutostartInfo{
		Enabled:    true,
		Command:    command,
		Executable: executable,
		ConfigPath: configPath,
	}, nil
}

func RemoveUserAgentAutostart() error {
	key, err := registry.OpenKey(registry.CURRENT_USER, windowsRunKey, registry.SET_VALUE|registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) || errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open Windows user Run key: %w", err)
	}
	defer key.Close()
	if err := key.DeleteValue(WindowsAgentRunValue); err != nil &&
		!errors.Is(err, registry.ErrNotExist) &&
		!errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		return fmt.Errorf("delete Windows user Run value: %w", err)
	}
	return nil
}
