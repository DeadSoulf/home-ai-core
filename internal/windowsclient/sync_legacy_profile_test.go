package windowsclient

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLegacyHTTPProfileDoesNotBlockNewHTTPSProfile(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "sync-profiles.json")
	now := time.Now().UTC().Truncate(time.Second)
	old := SyncProfile{
		ID: "sync-legacy", ServerURL: "http://dacha.alltech24.ru:8080",
		Username: "legacy", FolderID: "nsf-legacy",
		Source: filepath.Join(dir, "not-present"),
		Destination: "old/path", EverySeconds: 900, ConflictPolicy: SyncConflictStop,
		Enabled: true, CreatedAt: now, UpdatedAt: now, LastError: "previous error",
	}
	if err := persistSyncConfig(config, syncConfigState{
		Version: syncConfigVersion, Profiles: []SyncProfile{old},
	}); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}

	// Reading a legacy configuration never edits the source. It disables
	// remote plaintext profiles in memory so an existing agent cannot run them.
	loaded, err := LoadSyncProfiles(config)
	if err != nil {
		t.Fatalf("legacy profiles must remain visible and editable: %v", err)
	}
	if len(loaded) != 1 || loaded[0].Enabled ||
		loaded[0].ID != old.ID || loaded[0].ServerURL != old.ServerURL {
		t.Fatalf("legacy profile not safely quarantined: %#v", loaded)
	}
	if len(DueSyncProfiles(loaded, now.Add(time.Hour))) != 0 {
		t.Fatal("insecure HTTP profile was scheduled")
	}
	unchanged, err := os.ReadFile(config)
	if err != nil || !bytes.Equal(unchanged, original) {
		t.Fatal("read unexpectedly wrote the user's sync file")
	}

	source := filepath.Join(dir, "new-source")
	if err := os.Mkdir(source, 0o700); err != nil {
		t.Fatal(err)
	}
	added, err := AddSyncProfile(config, SyncProfileInput{
		ServerURL: "https://homeaicore.ru:82", Username: "texnik",
		FolderID: "nsf_new", Source: source, Destination: "001",
		Every: 15 * time.Minute, ConflictPolicy: SyncConflictStop,
	})
	if err != nil {
		t.Fatalf("could not add new HTTPS profile alongside legacy HTTP: %v", err)
	}
	if !added.Enabled || added.ServerURL != "https://homeaicore.ru:82" {
		t.Fatalf("new profile changed: %#v", added)
	}
	result, err := LoadSyncProfiles(config)
	if err != nil || len(result) != 2 || result[0].Enabled || !result[1].Enabled {
		t.Fatalf("profiles after migration = %#v, %v", result, err)
	}
	if result[0].ID != old.ID || result[0].ServerURL != old.ServerURL ||
		result[0].FolderID != old.FolderID || result[0].Source != old.Source ||
		result[0].LastError != old.LastError {
		t.Fatalf("old profile was modified beyond disabling: %#v", result[0])
	}
	backups, err := filepath.Glob(config + ".legacy-http-*.json")
	if err != nil || len(backups) != 1 {
		t.Fatalf("missing original legacy profile backup: %v, %v", backups, err)
	}
	backedUp, err := os.ReadFile(backups[0])
	if err != nil || !bytes.Equal(backedUp, original) {
		t.Fatal("legacy backup did not preserve the original file")
	}
	if err := SetSyncProfileEnabled(config, old.ID, true); err == nil ||
		!strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("legacy HTTP profile unexpectedly enabled: %v", err)
	}
	if _, err := AddSyncProfile(config, SyncProfileInput{
		ServerURL: old.ServerURL, Username: old.Username,
		FolderID: old.FolderID, Source: source, Destination: "safe",
		Every: 15 * time.Minute, ConflictPolicy: SyncConflictStop,
	}); err == nil {
		t.Fatal("creating a new remote HTTP profile must remain forbidden")
	}

	// Explicitly editing the legacy profile to a trusted HTTPS URL is allowed;
	// it remains disabled until the user chooses to re-enable it.
	modified, err := UpdateSyncProfile(config, old.ID, SyncProfileInput{
		ServerURL: "https://homeaicore.ru:82", Username: old.Username,
		FolderID: old.FolderID, Source: source, Destination: "repaired",
		Every: 15 * time.Minute, ConflictPolicy: SyncConflictStop,
	})
	if err != nil || modified.Enabled {
		t.Fatalf("legacy profile should be explicitly repairable but remain off: %#v, %v", modified, err)
	}
	if err := SetSyncProfileEnabled(config, old.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSyncProfiles(config); err != nil {
		t.Fatal(err)
	}
}

func TestMalformedLegacySyncProfileStillFailsClosed(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "profiles.json")
	now := time.Now().UTC().Truncate(time.Second)
	malformed := SyncProfile{
		ID: "sync-legacy", ServerURL: "http://user:password@home-ai.example:82",
		Username: "test", FolderID: "nsf",
		Source: filepath.Join(dir, "x"), Destination: "safe",
		EverySeconds: 900, ConflictPolicy: SyncConflictStop,
		Enabled: true, CreatedAt: now, UpdatedAt: now,
	}
	if err := persistSyncConfig(config, syncConfigState{
		Version: syncConfigVersion, Profiles: []SyncProfile{malformed},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSyncProfiles(config); err == nil {
		t.Fatal("credential-bearing remote HTTP profile should not be accepted")
	}
}
