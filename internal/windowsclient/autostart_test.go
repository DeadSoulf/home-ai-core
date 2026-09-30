package windowsclient

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildUserAgentCommand(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "Home AI", "client.exe")
	config := filepath.Join(root, "Profile Data", "sync.json")
	command, err := buildUserAgentCommand(executable, config)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(command, "agent run --config") ||
		!strings.Contains(command, executable) ||
		!strings.Contains(command, config) {
		t.Fatalf("command = %q", command)
	}
	for _, value := range []string{"relative.exe", filepath.Join(root, "bad\"name.exe")} {
		if _, err := buildUserAgentCommand(value, config); err == nil {
			t.Fatalf("accepted unsafe executable %q", value)
		}
	}
}

func TestAgentLockIsExclusiveAndReleases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.lock")
	first, err := AcquireAgentLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if second, err := AcquireAgentLock(path); err == nil {
		_ = second.Close()
		t.Fatal("second agent lock succeeded")
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	third, err := AcquireAgentLock(path)
	if err != nil {
		t.Fatalf("lock was not released: %v", err)
	}
	if err := third.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestParseUserAgentCommandRoundTrip(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "Home AI", "client.exe")
	config := filepath.Join(root, "Profile Data", "sync.json")
	command, err := buildUserAgentCommand(executable, config)
	if err != nil {
		t.Fatal(err)
	}
	gotExecutable, gotConfig, err := ParseUserAgentCommand(command)
	if err != nil {
		t.Fatal(err)
	}
	if gotExecutable != filepath.Clean(executable) || gotConfig != filepath.Clean(config) {
		t.Fatalf("parsed = %q, %q", gotExecutable, gotConfig)
	}
	for _, invalid := range []string{
		"",
		"client.exe agent run --config sync.json",
		"\"" + executable + "\" sync watch --config \"" + config + "\"",
		"\"" + executable + "\" agent run --config \"" + config + "\" extra",
	} {
		if _, _, err := ParseUserAgentCommand(invalid); err == nil {
			t.Fatalf("accepted invalid Run command %q", invalid)
		}
	}
}
