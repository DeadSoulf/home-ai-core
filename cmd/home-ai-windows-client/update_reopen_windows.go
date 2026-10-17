//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The verified cached installer is a different executable from the old
// running client. After it replaces the per-user install, it reopens the
// actual installed binary, not the old preview/downloaded binary.
func reopenWindowsClientSettings(executable, configPath, settingsPath string) error {
	if strings.TrimSpace(executable) == "" {
		return fmt.Errorf("installed Windows client path is required")
	}
	command := exec.Command(executable, windowsSettingsReopenArgs(configPath, settingsPath)...)
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: settingsCreateNoWindow}
	if err := command.Start(); err != nil {
		return fmt.Errorf("reopen updated Home-AI Windows client: %w", err)
	}
	if err := command.Process.Release(); err != nil {
		return fmt.Errorf("release updated Home-AI Windows client process: %w", err)
	}
	return nil
}

func notifyWindowsClientUpdateFailure(err error) {
	if err == nil {
		return
	}
	language := preferredUILanguage()
	base, _ := os.UserCacheDir()
	logPath := ""
	if base != "" {
		logPath = windowsClientUpdateLogPath(base)
	}
	message := textForLanguage(language, "update_install_failed") + "\n\n" +
		safeActivityText(err.Error(), 600)
	if logPath != "" {
		message += "\n\n" + textForLanguage(language, "update_log_location") + ": " + logPath
	}
	body, _ := windows.UTF16PtrFromString(message)
	title, _ := windows.UTF16PtrFromString(textForLanguage(language, "updates_title"))
	procSettingsMessageBox.Call(0, uintptr(unsafe.Pointer(body)),
		uintptr(unsafe.Pointer(title)), settingsMBOK|settingsMBIconError|settingsMBSetForeground)
}
