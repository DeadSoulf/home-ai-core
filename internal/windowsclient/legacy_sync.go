package windowsclient

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// IsLegacyRemoteHTTPServerURL identifies an old, unsupported HTTP endpoint
// saved by Windows clients before HTTPS became mandatory. This is only for
// displaying and safely disabling old profiles; never use it to connect.
func IsLegacyRemoteHTTPServerURL(raw string) bool {
	if strings.TrimSpace(raw) != raw {
		return false
	}
	address, err := url.Parse(raw)
	if err != nil || address.Scheme != "http" || address.Host == "" ||
		address.User != nil || address.Opaque != "" || address.Path != "" ||
		address.RawPath != "" || address.RawQuery != "" || address.ForceQuery ||
		address.Fragment != "" || address.RawFragment != "" {
		return false
	}
	host := address.Hostname()
	if host == "" || strings.EqualFold(host, "localhost") {
		return false
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return false
	}
	if port := address.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return false
		}
	}
	return true
}

// backupLegacySyncConfig preserves the exact original configuration before
// persisting a first-time migration. The original file remains in place until
// the atomic replacement succeeds. The caller must hold the sync config lock.
func backupLegacySyncConfig(filename string) (string, error) {
	source, err := os.Open(filename)
	if err != nil {
		return "", fmt.Errorf("open legacy sync profiles for backup: %w", err)
	}
	defer source.Close()
	backup, err := os.CreateTemp(filepath.Dir(filename), filepath.Base(filename)+".legacy-http-*.json")
	if err != nil {
		return "", fmt.Errorf("create legacy sync backup: %w", err)
	}
	name := backup.Name()
	success := false
	defer func() {
		if !success {
			_ = os.Remove(name)
		}
	}()
	if err := backup.Chmod(0o600); err != nil {
		_ = backup.Close()
		return "", err
	}
	written, err := io.Copy(backup, io.LimitReader(source, maxSyncConfigBytes+1))
	if err != nil {
		_ = backup.Close()
		return "", fmt.Errorf("copy legacy sync profiles: %w", err)
	}
	if written > maxSyncConfigBytes {
		_ = backup.Close()
		return "", errors.New("legacy sync profiles exceed backup size limit")
	}
	if err := backup.Sync(); err != nil {
		_ = backup.Close()
		return "", err
	}
	if err := backup.Close(); err != nil {
		return "", err
	}
	success = true
	return name, nil
}
