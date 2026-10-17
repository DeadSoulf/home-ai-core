//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
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
	// Important when the old GUI was launched from a preview outside the
	// installed path: copying the executable may succeed before the old GUI
	// has actually closed. Starting the new settings too early would focus
	// the old window and leave the user with no UI after it exits.
	if err := waitForWindowsSettingsWindowExit(20 * time.Second); err != nil {
		return err
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

func waitForWindowsSettingsWindowExit(timeout time.Duration) error {
	className, err := windows.UTF16PtrFromString(settingsWindowClass)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(timeout)
	for {
		hwnd, _, _ := procSettingsFindWindow.Call(uintptr(unsafe.Pointer(className)), 0)
		if hwnd == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("previous Home-AI settings window did not close; close it before reopening the updated client")
		}
		time.Sleep(100 * time.Millisecond)
	}
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
