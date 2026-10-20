//go:build windows

package main

import (
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procSettingsEnumWindows  = settingsUser32.NewProc("EnumWindows")
	procSettingsGetClassName = settingsUser32.NewProc("GetClassNameW")
	settingsEnumCallback     = syscall.NewCallback(findHomeAISettingsWindowCallback)
)

func isHomeAISettingsClass(name string) bool {
	return strings.EqualFold(name, settingsWindowClass)
}

// FindWindowW unexpectedly fails to discover a real top-level HOME AI
// Settings window on some Windows virtual-desktop configurations, even
// when EnumWindows lists it. Enumerate explicitly to restore a hidden
// window rather than spawning a duplicate GUI or leaving it inaccessible.
func findHomeAISettingsWindow() uintptr {
	var found uintptr
	procSettingsEnumWindows.Call(settingsEnumCallback, uintptr(unsafe.Pointer(&found)))
	return found
}

func findHomeAISettingsWindowCallback(hwnd, output uintptr) uintptr {
	var name [128]uint16
	count, _, _ := procSettingsGetClassName.Call(
		hwnd, uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)),
	)
	if count == 0 || !isHomeAISettingsClass(windows.UTF16ToString(name[:int(count)])) {
		return 1
	}
	*(*uintptr)(unsafe.Pointer(output)) = hwnd
	return 0
}
