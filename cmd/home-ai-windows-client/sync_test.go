package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/windowsclient"
)

func TestSyncCLIProfileLifecycleWithoutAuthentication(t *testing.T) {
	t.Setenv(passwordEnv, "")
	root := t.TempDir()
	source := filepath.Join(root, "source.txt")
	if err := os.WriteFile(source, []byte("sync"), 0o600); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(root, "sync.json")

	if err := run([]string{
		"sync", "add", "--config", config,
		"--server", "http://127.0.0.1:1", "--username", "alice", "--folder", "nsf_test",
		"--source", source, "--dest", "backup/source.txt", "--every", "2m", "--conflict", "skip",
	}); err != nil {
		t.Fatal(err)
	}
	profiles, err := windowsclient.LoadSyncProfiles(config)
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 || profiles[0].ConflictPolicy != windowsclient.SyncConflictSkip ||
		profiles[0].Interval() != 2*time.Minute {
		t.Fatalf("profiles = %#v", profiles)
	}
	id := profiles[0].ID
	if err := run([]string{"sync", "list", "--config", config}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"sync", "disable", "--config", config, "--profile", id}); err != nil {
		t.Fatal(err)
	}
	profiles, err = windowsclient.LoadSyncProfiles(config)
	if err != nil || profiles[0].Enabled {
		t.Fatalf("disabled profile = %#v, %v", profiles, err)
	}
	if err := run([]string{"sync", "enable", "--config", config, "--profile", id}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"sync", "remove", "--config", config, "--profile", id}); err != nil {
		t.Fatal(err)
	}
	profiles, err = windowsclient.LoadSyncProfiles(config)
	if err != nil || len(profiles) != 0 {
		t.Fatalf("profiles after remove = %#v, %v", profiles, err)
	}
}

func TestSyncCLIRunRequiresProcessPassword(t *testing.T) {
	t.Setenv(passwordEnv, "")
	root := t.TempDir()
	source := filepath.Join(root, "source.txt")
	if err := os.WriteFile(source, []byte("sync"), 0o600); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(root, "sync.json")
	if err := run([]string{
		"sync", "add", "--config", config,
		"--server", "http://127.0.0.1:1", "--username", "alice", "--folder", "nsf_test",
		"--source", source,
	}); err != nil {
		t.Fatal(err)
	}
	profiles, err := windowsclient.LoadSyncProfiles(config)
	if err != nil {
		t.Fatal(err)
	}
	err = run([]string{"sync", "run", "--config", config, "--profile", profiles[0].ID})
	if err == nil || !strings.Contains(err.Error(), passwordEnv) {
		t.Fatalf("missing process password error = %v", err)
	}
}

func TestSyncCLIValidation(t *testing.T) {
	for _, args := range [][]string{
		{"sync"},
		{"sync", "unknown"},
		{"sync", "remove"},
		{"sync", "run", "--retries", "0"},
		{"sync", "watch", "--poll", "100ms"},
		{"sync", "add", "--server", "http://127.0.0.1:1"},
	} {
		if err := run(args); err == nil {
			t.Fatalf("run(%v) succeeded", args)
		}
	}
}

func TestWatchSyncProfilesRunNowUsesSameSchedulerLoop(t *testing.T) {
	t.Setenv(passwordEnv, "test-password")
	root := t.TempDir()
	source := filepath.Join(root, "source.txt")
	if err := os.WriteFile(source, []byte("sync"), 0o600); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(root, "sync.json")
	profile, err := windowsclient.AddSyncProfile(config, windowsclient.SyncProfileInput{
		ServerURL:      "http://127.0.0.1:1",
		Username:       "alice",
		FolderID:       "nsf_test",
		Source:         source,
		Destination:    "backup/source.txt",
		Every:          time.Minute,
		ConflictPolicy: windowsclient.SyncConflictStop,
	})
	if err != nil {
		t.Fatal(err)
	}
	initial := time.Now().UTC()
	if err := windowsclient.RecordSyncProfileResult(config, profile.ID, initial, nil); err != nil {
		t.Fatal(err)
	}
	runNow := make(chan struct{}, 1)
	runNow <- struct{}{}
	ctx, cancel := context.WithTimeout(context.Background(), 750*time.Millisecond)
	defer cancel()
	if err := watchSyncProfiles(ctx, config, "", 1, false, time.Second, runNow, nil); err != nil {
		t.Fatal(err)
	}
	profiles, err := windowsclient.LoadSyncProfiles(config)
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 || profiles[0].LastAttemptAt == nil ||
		!profiles[0].LastAttemptAt.After(initial) || profiles[0].LastError == "" {
		t.Fatalf("trigger did not execute a forced sync: %#v", profiles)
	}
}
