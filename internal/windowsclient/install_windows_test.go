//go:build windows

package windowsclient

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUserClientInstallUsesLocalAppDataStablePath(t *testing.T) {
	local := t.TempDir()
	t.Setenv("LOCALAPPDATA", local)
	source := filepath.Join(t.TempDir(), "downloaded.exe")
	if err := os.WriteFile(source, []byte("client-bytes"), 0o700); err != nil {
		t.Fatal(err)
	}
	path, err := InstallUserClient(source)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(local, "HomeAI", "bin", InstalledWindowsClientName)
	if path != want {
		t.Fatalf("install path = %q, want %q", path, want)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "client-bytes" {
		t.Fatalf("installed data = %q, err %v", data, err)
	}
	statusPath, installed, err := UserClientInstallStatus()
	if err != nil || !installed || statusPath != want {
		t.Fatalf("status = %q, %v, %v", statusPath, installed, err)
	}
}
