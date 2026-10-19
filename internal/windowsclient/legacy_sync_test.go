package windowsclient

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsLegacyRemoteHTTPServerURL(t *testing.T) {
	for _, raw := range []string{
		"http://dacha.alltech24.ru:8080",
		"http://192.168.1.44:8080",
		"http://home-ai.example",
	} {
		if !IsLegacyRemoteHTTPServerURL(raw) {
			t.Errorf("unrecognized legacy URL %q", raw)
		}
	}
	for _, raw := range []string{
		"", "localhost", "http://localhost:8080", "http://127.0.0.1:8080",
		"http://[::1]:8080", "https://homeaicore.ru:82",
		"http://user:pass@home-ai.example:82", "http://home-ai.example/path",
		"http://home-ai.example?token=x", "http://home-ai.example/#fragment",
		"http://home-ai.example:0", "http://home-ai.example:65536",
		"http://home-ai.example:hello", " http://home-ai.example ",
	} {
		if IsLegacyRemoteHTTPServerURL(raw) {
			t.Errorf("accepted unexpected legacy URL %q", raw)
		}
	}
}

func TestBackupLegacySyncConfigCopiesOriginalAndDoesNotModifyIt(t *testing.T) {
	dir := t.TempDir()
	original := filepath.Join(dir, "sync-profiles.json")
	data := []byte("{\"version\":1,\"profiles\":[{\"server_url\":\"http://old.example\"}]}")
	if err := os.WriteFile(original, data, 0o600); err != nil {
		t.Fatal(err)
	}
	backup, err := backupLegacySyncConfig(original)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(filepath.Base(backup), "legacy-http-") {
		t.Fatalf("unexpected backup filename %q", backup)
	}
	for _, name := range []string{original, backup} {
		got, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(data) {
			t.Fatalf("%s was modified: %q", name, got)
		}
	}
}
