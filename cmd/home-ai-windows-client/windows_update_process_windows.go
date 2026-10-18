//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/windowsclient"
)

// The update log must remain readable after the old GUI closes, and no
// credential or Home-AI authentication token is ever passed to a child.
func windowsClientUpdateLogPath(cacheRoot string) string {
	return filepath.Join(cacheRoot, "HomeAI", "windows-client-update.log")
}

func openWindowsClientUpdateLog() (*os.File, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("locate update log directory: %w", err)
	}
	path := windowsClientUpdateLogPath(base)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create update log directory: %w", err)
	}
	return os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
}

func startLoggedWindowsUpdateChild(log *os.File, executable string, args ...string) error {
	cmd := exec.Command(executable, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: settingsCreateNoWindow}
	cmd.Stdout = log
	cmd.Stderr = log
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launch Windows client updater: %w", err)
	}
	return cmd.Process.Release()
}

// Used by the tray's update action. The new verified installer, including
// one launched by an older client, now reopens the installed UI upon success.
func launchWindowsClientUpdate(channel string) error {
	channel, err := windowsclient.NormalizeReleaseChannel(channel)
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	log, err := openWindowsClientUpdateLog()
	if err != nil {
		return err
	}
	defer log.Close()
	_, _ = fmt.Fprintf(log, "%s tray-initiated %s update\n", time.Now().Format(time.RFC3339), channel)
	return startLoggedWindowsUpdateChild(log, executable, "client", "update", "--channel", channel)
}

// The GUI has already downloaded and SHA-256 verified this executable while
// it was still visible. Only now does it launch the independent installer and
// release its own executable lock.
func launchVerifiedWindowsClientInstall(executable, version string) error {
	info, err := os.Lstat(executable)
	if err != nil {
		return fmt.Errorf("inspect verified client executable: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("verified client executable is not a real regular file")
	}
	log, err := openWindowsClientUpdateLog()
	if err != nil {
		return err
	}
	defer log.Close()
	_, _ = fmt.Fprintf(log, "%s GUI update %s downloaded and verified; starting installer\n",
		time.Now().Format(time.RFC3339), version)
	if err := startLoggedWindowsUpdateChild(log, executable, "client", "install"); err != nil {
		_, _ = fmt.Fprintf(log, "installer launch failed: %v\n", err)
		return err
	}
	return nil
}
