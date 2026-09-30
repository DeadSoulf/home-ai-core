//go:build windows

package windowsclient

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	windowsAgentTrayClass = "HomeAIWindowsSyncTray"
	windowsAgentTrayTitle = "Home-AI Sync Agent"
	windowsWMClose        = 0x0010
)

var (
	user32AgentControlDLL = windows.NewLazySystemDLL("user32.dll")
	procFindWindowW       = user32AgentControlDLL.NewProc("FindWindowW")
	procPostMessageAgentW = user32AgentControlDLL.NewProc("PostMessageW")
)

func RequestUserAgentExit() (bool, error) {
	className, err := windows.UTF16PtrFromString(windowsAgentTrayClass)
	if err != nil {
		return false, err
	}
	windowName, err := windows.UTF16PtrFromString(windowsAgentTrayTitle)
	if err != nil {
		return false, err
	}
	hwnd, _, callErr := procFindWindowW.Call(
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowName)),
	)
	if hwnd == 0 {
		if callErr != nil && !errors.Is(callErr, windows.ERROR_SUCCESS) {
			return false, fmt.Errorf("find Home-AI tray agent: %w", callErr)
		}
		return false, nil
	}
	ok, _, callErr := procPostMessageAgentW.Call(hwnd, windowsWMClose, 0, 0)
	if ok == 0 {
		if callErr == nil || errors.Is(callErr, windows.ERROR_SUCCESS) {
			callErr = syscall.EINVAL
		}
		return false, fmt.Errorf("request Home-AI tray agent exit: %w", callErr)
	}
	return true, nil
}

func StartUserAgent(executable, configPath string) error {
	if _, err := buildUserAgentCommand(executable, configPath); err != nil {
		return err
	}
	command := exec.Command(executable, "agent", "run", "--config", configPath)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := command.Start(); err != nil {
		return fmt.Errorf("start Home-AI tray agent: %w", err)
	}
	if err := command.Process.Release(); err != nil {
		return fmt.Errorf("release Home-AI tray agent process: %w", err)
	}
	return nil
}

func UserAgentRunning() bool {
	className, err := windows.UTF16PtrFromString(windowsAgentTrayClass)
	if err != nil {
		return false
	}
	windowName, err := windows.UTF16PtrFromString(windowsAgentTrayTitle)
	if err != nil {
		return false
	}
	hwnd, _, _ := procFindWindowW.Call(
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowName)),
	)
	return hwnd != 0
}

func SignalUserAgentSyncNow() (bool, error) {
	className, err := windows.UTF16PtrFromString(windowsAgentTrayClass)
	if err != nil {
		return false, err
	}
	windowName, err := windows.UTF16PtrFromString(windowsAgentTrayTitle)
	if err != nil {
		return false, err
	}
	hwnd, _, callErr := procFindWindowW.Call(
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowName)),
	)
	if hwnd == 0 {
		if callErr != nil && !errors.Is(callErr, windows.ERROR_SUCCESS) {
			return false, fmt.Errorf("find Home-AI tray agent: %w", callErr)
		}
		return false, nil
	}
	const (
		wmCommand     = 0x0111
		traySyncNowID = 1001
	)
	result, _, callErr := procPostMessageAgentW.Call(hwnd, wmCommand, traySyncNowID, 0)
	if result == 0 {
		if callErr == nil || errors.Is(callErr, windows.ERROR_SUCCESS) {
			callErr = syscall.EINVAL
		}
		return false, fmt.Errorf("signal Home-AI sync agent: %w", callErr)
	}
	return true, nil
}
