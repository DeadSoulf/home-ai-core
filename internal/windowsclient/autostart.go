package windowsclient

import (
	"errors"
	"path/filepath"
	"strings"
)

const WindowsAgentRunValue = "HomeAIWindowsSyncAgent"

var ErrUserAutostartUnsupported = errors.New("Windows user autostart is unavailable on this platform")

type UserAutostartInfo struct {
	Enabled bool
	Command string
}

func buildUserAgentCommand(executable, configPath string) (string, error) {
	executable = filepath.Clean(strings.TrimSpace(executable))
	configPath = filepath.Clean(strings.TrimSpace(configPath))
	if executable == "." || configPath == "." || !filepath.IsAbs(executable) || !filepath.IsAbs(configPath) {
		return "", errors.New("agent executable and sync config paths must be absolute")
	}
	for _, value := range []string{executable, configPath} {
		if strings.Contains(value, """) || strings.ContainsAny(value, "\r\n") {
			return "", errors.New("agent paths cannot contain quotes or control characters")
		}
	}
	return """ + executable + "" agent run --config "" + configPath + """, nil
}
