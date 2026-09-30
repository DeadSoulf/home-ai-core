//go:build windows

package main

import (
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	localeKernel32              = windows.NewLazySystemDLL("kernel32.dll")
	procGetUserDefaultLocaleName = localeKernel32.NewProc("GetUserDefaultLocaleName")
)

func defaultUILanguage() string {
	var buffer [85]uint16
	result, _, _ := procGetUserDefaultLocaleName.Call(
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(len(buffer)),
	)
	if result == 0 {
		return uiLanguageEnglish
	}
	locale := strings.ToLower(windows.UTF16ToString(buffer[:]))
	if strings.HasPrefix(locale, "ru") {
		return uiLanguageRussian
	}
	return uiLanguageEnglish
}
