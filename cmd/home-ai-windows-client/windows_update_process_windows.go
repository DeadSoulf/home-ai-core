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

func launchWindowsClientUpdate(channel string) error {
	channel, err := windowsclient.NormalizeReleaseChannel(channel)
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return err
	}
	logPath := filepath.Join(base, "HomeAI", "windows-client-update.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return err
	}
	log, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer log.Close()
	_, _ = fmt.Fprintf(log, "%s user-initiated %s update\n", time.Now().Format(time.RFC3339), channel)
	cmd := exec.Command(executable, "client", "update", "--channel", channel)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: settingsCreateNoWindow}
	cmd.Stdout = log
	cmd.Stderr = log
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
