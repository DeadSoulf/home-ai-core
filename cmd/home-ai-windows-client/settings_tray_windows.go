//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"github.com/DeadSoulf/home-ai-core/internal/windowsclient"
	"golang.org/x/sys/windows"
)

const trayStartupTimeout = 8 * time.Second

type settingsTrayMinimizeResult struct {
	err error
}

// A settings window must never disappear unless a separate, running tray
// agent is available to restore it. Autostart registration alone does not
// mean the tray icon is running in the current Windows session.
func ensureTrayAgentForSettings(configPath string) error {
	if windowsclient.UserAgentRunning() {
		return nil
	}
	info, err := windowsclient.UserAgentAutostartStatus()
	if err != nil {
		return fmt.Errorf("check Home-AI tray autostart: %w", err)
	}
	if !info.Enabled {
		return errors.New("Home-AI tray agent is not enabled; enable the background agent in Settings first")
	}
	if !sameTraySyncConfig(configPath, info.ConfigPath) {
		return errors.New("tray agent uses a different sync-profile configuration")
	}
	executable := strings.TrimSpace(info.Executable)
	if executable == "" {
		return errors.New("Home-AI tray agent executable path is empty")
	}
	if file, err := os.Stat(executable); err != nil || !file.Mode().IsRegular() {
		return errors.New("Home-AI tray agent executable does not exist or is not a regular file")
	}
	// An old client may have registered autostart but left the agent stopped
	// until the next interactive Windows logon. Start that exact registered
	// agent now, without modifying profiles, update channels or credentials.
	if err := windowsclient.StartUserAgent(executable, info.ConfigPath); err != nil {
		return fmt.Errorf("start Home-AI tray agent: %w", err)
	}
	deadline := time.Now().Add(trayStartupTimeout)
	for time.Now().Before(deadline) {
		if windowsclient.UserAgentRunning() {
			// The tray window is created slightly before Shell_NotifyIcon.
			// Confirm it survives a short interval before hiding Settings.
			time.Sleep(250 * time.Millisecond)
			if windowsclient.UserAgentRunning() {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("Home-AI tray icon did not start; Settings remains in the taskbar")
}

func sameTraySyncConfig(left, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" || right == "" {
		return false
	}
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}

func (state *windowsSettingsUI) requestMinimizeToTray() {
	if state.trayMinimizePending {
		return
	}
	if windowsclient.UserAgentRunning() {
		state.hideToTray()
		return
	}
	state.trayMinimizePending = true
	state.setStatus(state.tr("tray_starting"))
	hwnd := uintptr(state.hwnd)
	configPath := state.configPath
	go func() {
		err := ensureTrayAgentForSettings(configPath)
		state.asyncMu.Lock()
		state.trayMinimizeResult = &settingsTrayMinimizeResult{err: err}
		state.asyncMu.Unlock()
		procSettingsPostMessage.Call(hwnd, settingsWMTrayMinimizeDone, 0, 0)
	}()
}

func (state *windowsSettingsUI) finishMinimizeToTray() {
	state.asyncMu.Lock()
	result := state.trayMinimizeResult
	state.trayMinimizeResult = nil
	state.asyncMu.Unlock()
	if result == nil {
		return
	}
	state.trayMinimizePending = false
	if result.err != nil {
		// The minimize button keeps its ordinary Windows taskbar behavior.
		// A failure must never strand the application in an invisible window.
		state.setStatus(state.tr("tray_unavailable") + ": " + result.err.Error())
		return
	}
	minimized, _, _ := procSettingsIsIconic.Call(uintptr(state.hwnd))
	if minimized == 0 || !windowsclient.UserAgentRunning() {
		return // User restored Settings during startup, or agent exited.
	}
	state.hideToTray()
}

func (state *windowsSettingsUI) hideToTray() {
	if !windowsclient.UserAgentRunning() {
		return
	}
	procSettingsShowWindow.Call(uintptr(state.hwnd), settingsSWHide)
	state.trayHidden = true
}

// If the agent exits or fails after hiding Settings, restore the taskbar
// window automatically so there is always a way back into the application.
func (state *windowsSettingsUI) restoreIfTrayStopped() {
	if !state.trayHidden || windowsclient.UserAgentRunning() {
		return
	}
	state.trayHidden = false
	procSettingsShowWindow.Call(uintptr(state.hwnd), settingsSWRestore)
	state.setStatus(state.tr("tray_unavailable"))
}

// Reuse the existing settings window instead of spawning another process on
// every tray double click. A hidden/minimized window is restored by the
// existing focusExistingSettingsWindow implementation.
func showSettingsFromTray(configPath string) {
	if !focusExistingSettingsWindow() {
		_ = startSettingsProcess(configPath)
	}
}

func closeHiddenSettingsWhenTrayExits() {
	className, _ := windows.UTF16PtrFromString(settingsWindowClass)
	hwnd, _, _ := procSettingsFindWindow.Call(uintptr(unsafe.Pointer(className)), 0)
	if hwnd == 0 {
		return
	}
	visible, _, _ := procSettingsIsWindowVisible.Call(hwnd)
	if visible == 0 {
		procSettingsPostMessage.Call(hwnd, settingsWMClose, 0, 0)
	}
}
