//go:build windows

package main

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

var (
	kernel32ConsoleDLL   = windows.NewLazySystemDLL("kernel32.dll")
	procGetConsoleWindow = kernel32ConsoleDLL.NewProc("GetConsoleWindow")
	user32ConsoleDLL     = windows.NewLazySystemDLL("user32.dll")
	procShowWindow       = user32ConsoleDLL.NewProc("ShowWindow")
)

func prepareAgentRuntime(logPath string) (func() error, error) {
	file, err := openAgentLog(logPath)
	if err != nil {
		return nil, err
	}
	originalOut, originalErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = file, file

	if window, _, _ := procGetConsoleWindow.Call(); window != 0 {
		procShowWindow.Call(window, 0)
	}
	cleanup := func() error {
		os.Stdout, os.Stderr = originalOut, originalErr
		if err := file.Sync(); err != nil {
			_ = file.Close()
			return fmt.Errorf("sync agent log: %w", err)
		}
		return file.Close()
	}
	return cleanup, nil
}
