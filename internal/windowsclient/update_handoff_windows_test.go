//go:build windows

package windowsclient

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestApplyUserClientUpdateReplacesStableInstalledCopy(t *testing.T) {
	local := t.TempDir()
	t.Setenv("LOCALAPPDATA", local)
	oldSource := filepath.Join(t.TempDir(), "old.exe")
	if err := os.WriteFile(oldSource, []byte("old-client"), 0o700); err != nil {
		t.Fatal(err)
	}
	target, err := InstallUserClient(oldSource)
	if err != nil {
		t.Fatal(err)
	}
	updateDir, err := UserClientUpdateDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(updateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	candidate := filepath.Join(updateDir, "home-ai-windows-client_0.1.80-dev_amd64.exe")
	payload := []byte("new-client")
	if err := os.WriteFile(candidate, payload, 0o700); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	update := DownloadedWindowsClientUpdate{
		Version: "0.1.80-dev",
		Path:    candidate,
		SHA256:  hex.EncodeToString(sum[:]),
	}
	if err := ApplyUserClientUpdate(update, 0, ""); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(payload) {
		t.Fatalf("installed bytes = %q", data)
	}
	if _, err := os.Stat(candidate); !os.IsNotExist(err) {
		t.Fatalf("candidate was not removed after apply: %v", err)
	}
}

func TestApplyUserClientUpdateRejectsCandidateOutsideUpdateDir(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	candidate := filepath.Join(t.TempDir(), "client.exe")
	payload := []byte("candidate")
	if err := os.WriteFile(candidate, payload, 0o700); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	err := ApplyUserClientUpdate(DownloadedWindowsClientUpdate{
		Version: "0.1.80-dev",
		Path:    candidate,
		SHA256:  hex.EncodeToString(sum[:]),
	}, 0, "")
	if err == nil {
		t.Fatal("accepted candidate outside update directory")
	}
}
