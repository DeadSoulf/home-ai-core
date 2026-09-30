package windowsclient

import (
	"path/filepath"
	"testing"
)

func TestClientSettingsRoundTrip(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "settings.json")
	want := ClientSettings{
		Version:   1,
		ServerURL: "http://home-ai.local:8080",
		Username:  "alice",
	}
	if err := SaveClientSettings(filename, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadClientSettings(filename)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("settings = %#v, want %#v", got, want)
	}
}

func TestLoadClientSettingsMissingReturnsEmptyDefaults(t *testing.T) {
	got, err := LoadClientSettings(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != 1 || got.ServerURL != "" || got.Username != "" {
		t.Fatalf("unexpected defaults: %#v", got)
	}
}
