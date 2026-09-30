//go:build windows

package windowsclient

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsUserAgentAutostartRoundTrip(t *testing.T) {
	_ = RemoveUserAgentAutostart()
	t.Cleanup(func() { _ = RemoveUserAgentAutostart() })

	root := t.TempDir()
	executable := filepath.Join(root, "Home AI Client.exe")
	config := filepath.Join(root, "sync profiles.json")
	if err := InstallUserAgentAutostart(executable, config); err != nil {
		t.Fatal(err)
	}
	info, err := UserAgentAutostartStatus()
	if err != nil {
		t.Fatal(err)
	}
	if !info.Enabled ||
		!strings.Contains(info.Command, executable) ||
		!strings.Contains(info.Command, config) ||
		!strings.Contains(info.Command, "agent run --config") {
		t.Fatalf("autostart info = %#v", info)
	}
	if err := RemoveUserAgentAutostart(); err != nil {
		t.Fatal(err)
	}
	info, err = UserAgentAutostartStatus()
	if err != nil {
		t.Fatal(err)
	}
	if info.Enabled {
		t.Fatalf("autostart remained enabled: %#v", info)
	}
}
