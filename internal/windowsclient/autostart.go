package windowsclient

import (
	"errors"
	"path/filepath"
	"strings"
)

const WindowsAgentRunValue = "HomeAIWindowsSyncAgent"

var ErrUserAutostartUnsupported = errors.New("Windows user autostart is unavailable on this platform")

type UserAutostartInfo struct {
	Enabled    bool
	Command    string
	Executable string
	ConfigPath string
}

func buildUserAgentCommand(executable, configPath string) (string, error) {
	executable = filepath.Clean(strings.TrimSpace(executable))
	configPath = filepath.Clean(strings.TrimSpace(configPath))
	if executable == "." || configPath == "." || !filepath.IsAbs(executable) || !filepath.IsAbs(configPath) {
		return "", errors.New("agent executable and sync config paths must be absolute")
	}
	for _, value := range []string{executable, configPath} {
		if strings.Contains(value, "\"") || strings.ContainsAny(value, "\r\n") {
			return "", errors.New("agent paths cannot contain quotes or control characters")
		}
	}
	return "\"" + executable + "\" agent run --config \"" + configPath + "\"", nil
}


func ParseUserAgentCommand(command string) (string, string, error) {
	command = strings.TrimSpace(command)
	if !strings.HasPrefix(command, """) {
		return "", "", errors.New("agent Run command must start with a quoted executable")
	}
	command = command[1:]
	executableEnd := strings.IndexByte(command, '"')
	if executableEnd < 0 {
		return "", "", errors.New("agent Run command executable quote is not closed")
	}
	executable := command[:executableEnd]
	remainder := strings.TrimSpace(command[executableEnd+1:])
	const prefix = "agent run --config ""
	if !strings.HasPrefix(remainder, prefix) || !strings.HasSuffix(remainder, """) {
		return "", "", errors.New("agent Run command has an unexpected format")
	}
	configPath := strings.TrimSuffix(strings.TrimPrefix(remainder, prefix), """)
	if strings.Contains(configPath, """) {
		return "", "", errors.New("agent Run command config path contains an unexpected quote")
	}
	rebuilt, err := buildUserAgentCommand(executable, configPath)
	if err != nil {
		return "", "", err
	}
	if rebuilt != """ + executable + "" " + remainder {
		return "", "", errors.New("agent Run command is not canonical")
	}
	return filepath.Clean(executable), filepath.Clean(configPath), nil
}
