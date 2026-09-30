//go:build windows

package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/windowsclient"
	"golang.org/x/sys/windows"
)

func TestWindowsClientInstallHandoffClosesTrayAndReplacesLockedTarget(t *testing.T) {
	local := t.TempDir()
	t.Setenv("LOCALAPPDATA", local)

	target, err := windowsclient.UserClientInstallPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("old-client"), 0o700); err != nil {
		t.Fatal(err)
	}
	targetPtr, err := windows.UTF16PtrFromString(target)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(
		targetPtr,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		t.Fatal(err)
	}
	handleOpen := true
	defer func() {
		if handleOpen {
			_ = windows.CloseHandle(handle)
		}
	}()

	config := filepath.Join(t.TempDir(), "sync.json")
	logPath := filepath.Join(t.TempDir(), "agent.log")
	tray, err := startAgentTray(logPath, config)
	if err != nil {
		t.Fatal(err)
	}
	defer tray.Close()

	released := make(chan struct{})
	go func() {
		<-tray.Exit
		_ = windows.CloseHandle(handle)
		handleOpen = false
		close(released)
	}()

	source := filepath.Join(t.TempDir(), "new-client.exe")
	if err := os.WriteFile(source, []byte("new-client"), 0o700); err != nil {
		t.Fatal(err)
	}
	result, err := windowsclient.InstallUserClientWithHandoff(source, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed || !result.AgentExitRequested || result.Path != target {
		t.Fatalf("handoff result = %#v", result)
	}
	select {
	case <-released:
	case <-time.After(2 * time.Second):
		t.Fatal("tray exit did not release the locked client")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new-client" {
		t.Fatalf("installed client = %q", data)
	}
}
