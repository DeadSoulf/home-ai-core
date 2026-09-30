package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/windowsclient"
)

func TestAgentTraySummaryTracksLatestEnabledProfile(t *testing.T) {
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
	if summary := agentTraySummary(config, time.Now()); !strings.Contains(summary, "1 enabled") ||
		!strings.Contains(summary, "not synced yet") {
		t.Fatalf("initial summary = %q", summary)
	}
	attempt := time.Date(2026, 9, 30, 18, 15, 0, 0, time.UTC)
	if err := windowsclient.RecordSyncProfileResult(config, profile.ID, attempt, nil); err != nil {
		t.Fatal(err)
	}
	if summary := agentTraySummary(config, attempt); !strings.Contains(summary, "last OK") {
		t.Fatalf("success summary = %q", summary)
	}
	if err := windowsclient.RecordSyncProfileResult(config, profile.ID, attempt.Add(time.Minute), errors.New("network failed")); err != nil {
		t.Fatal(err)
	}
	if summary := agentTraySummary(config, attempt.Add(time.Minute)); !strings.Contains(summary, "last FAILED") {
		t.Fatalf("failure summary = %q", summary)
	}
}
