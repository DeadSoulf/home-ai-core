package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTransferActivityRoundTrip(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "sync-profiles.json")
	want := transferActivity{
		ProfileID: "sync-test", Path: "Backups/example.txt", Status: "running",
		UploadedBytes: 1024, TotalBytes: 2048, ItemsDone: 1, ItemsTotal: 3,
	}
	if err := saveTransferProgress(configPath, want); err != nil {
		t.Fatal(err)
	}
	got, err := readTransferProgress(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if got.ProfileID != want.ProfileID || got.Status != want.Status ||
		got.UploadedBytes != 1024 || got.TotalBytes != 2048 || got.At.IsZero() {
		t.Fatalf("unexpected progress: %+v", got)
	}

	if err := appendTransferHistory(configPath, transferActivity{Path: "first", Status: "copied"}); err != nil {
		t.Fatal(err)
	}
	if err := appendTransferHistory(configPath, transferActivity{Path: "second", Status: "skipped"}); err != nil {
		t.Fatal(err)
	}
	history, err := readTransferHistory(configPath, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].Path != "second" {
		t.Fatalf("most recent history incorrect: %+v", history)
	}
	current, logfile := transferActivityPaths(configPath)
	for _, path := range []string{current, logfile} {
		info, err := os.Stat(path)
		if err != nil || info.Size() == 0 {
			t.Fatalf("activity checkpoint %s: %v", path, err)
		}
	}
}

func TestSafeTransferActivityText(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "sync.json")
	if err := appendTransferHistory(configPath, transferActivity{
		Path: "A\nB\rC", Error: strings.Repeat("x", 700), Status: "failed",
	}); err != nil {
		t.Fatal(err)
	}
	items, err := readTransferHistory(configPath, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Path != "A B C" || len([]rune(items[0].Error)) > 513 {
		t.Fatalf("unsafe log record: %+v", items)
	}
}
