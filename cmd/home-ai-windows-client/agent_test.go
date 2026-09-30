package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAgentLogRotation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.log")
	if err := os.WriteFile(path, make([]byte, maxAgentLogBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := openAgentLog(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("new log\n"); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("rotated log missing: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new log\n" {
		t.Fatalf("new log = %q", data)
	}
}
