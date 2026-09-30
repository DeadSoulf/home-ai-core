package windowsclient

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	clientSettingsVersion  = 1
	maxClientSettingsBytes = 64 << 10
)

type ClientSettings struct {
	Version   int    `json:"version"`
	ServerURL string `json:"server_url,omitempty"`
	Username  string `json:"username,omitempty"`
	Language  string `json:"language,omitempty"`
}

func LoadClientSettings(filename string) (ClientSettings, error) {
	filename, err := normalizeClientSettingsFilename(filename)
	if err != nil {
		return ClientSettings{}, err
	}
	settings := ClientSettings{Version: clientSettingsVersion}
	file, err := os.Open(filename)
	if errors.Is(err, os.ErrNotExist) {
		return settings, nil
	}
	if err != nil {
		return ClientSettings{}, fmt.Errorf("open client settings: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return ClientSettings{}, fmt.Errorf("inspect client settings: %w", err)
	}
	if info.Size() > maxClientSettingsBytes {
		return ClientSettings{}, errors.New("client settings file exceeds 64 KiB")
	}
	decoder := json.NewDecoder(io.LimitReader(file, maxClientSettingsBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&settings); err != nil {
		return ClientSettings{}, fmt.Errorf("decode client settings: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return ClientSettings{}, errors.New("client settings file must contain exactly one JSON object")
	}
	if err := validateClientSettings(settings); err != nil {
		return ClientSettings{}, fmt.Errorf("invalid client settings: %w", err)
	}
	return settings, nil
}

func SaveClientSettings(filename string, settings ClientSettings) error {
	filename, err := normalizeClientSettingsFilename(filename)
	if err != nil {
		return err
	}
	settings.Version = clientSettingsVersion
	if err := validateClientSettings(settings); err != nil {
		return err
	}
	lock, err := lockQueueFile(filename + ".lock")
	if err != nil {
		return fmt.Errorf("lock client settings: %w", err)
	}
	defer func() {
		_ = unlockQueueFile(lock)
		_ = lock.Close()
	}()

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("encode client settings: %w", err)
	}
	data = append(data, '\n')
	file, err := os.CreateTemp(filepath.Dir(filename), ".home-ai-settings-*")
	if err != nil {
		return fmt.Errorf("create client settings checkpoint: %w", err)
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write client settings checkpoint: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync client settings checkpoint: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close client settings checkpoint: %w", err)
	}
	if err := replaceQueueFile(name, filename); err != nil {
		return fmt.Errorf("replace client settings checkpoint: %w", err)
	}
	return nil
}

func normalizeClientSettingsFilename(filename string) (string, error) {
	if strings.TrimSpace(filename) == "" {
		return "", errors.New("client settings path is required")
	}
	filename, err := filepath.Abs(filename)
	if err != nil {
		return "", fmt.Errorf("resolve client settings path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
		return "", fmt.Errorf("create client settings directory: %w", err)
	}
	return filename, nil
}

func validateClientSettings(settings ClientSettings) error {
	if settings.Version != clientSettingsVersion {
		return fmt.Errorf("unsupported client settings version %d", settings.Version)
	}
	if settings.Language != "" && settings.Language != "en" && settings.Language != "ru" {
		return errors.New("unsupported client language")
	}
	if settings.ServerURL == "" && settings.Username == "" {
		return nil
	}
	server, err := normalizeQueueServer(settings.ServerURL)
	if err != nil || server != settings.ServerURL {
		return errors.New("invalid server URL")
	}
	if strings.TrimSpace(settings.Username) == "" || strings.TrimSpace(settings.Username) != settings.Username {
		return errors.New("invalid username")
	}
	return nil
}
