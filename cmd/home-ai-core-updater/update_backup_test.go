package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeBackupTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readBackupTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestStagedRollbackContainsCompleteCurrentSnapshot(t *testing.T) {
	root := t.TempDir()
	live := filepath.Join(root, "live")
	core := filepath.Join(live, "bin", "home-ai-core")
	web := filepath.Join(live, "web")
	helper := filepath.Join(live, "libexec", "home-ai-core-updater")
	writeBackupTestFile(t, core, "working-core-174")
	writeBackupTestFile(t, filepath.Join(web, "index.html"), "working-web-174")
	writeBackupTestFile(t, helper, "working-helper-174")

	staged, err := stageRollbackBackup(root, core, web, helper, "0.1.174-dev", "0.1.175-dev", true)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(staged)

	if got := readBackupTestFile(t, filepath.Join(staged, "bin", "home-ai-core")); got != "working-core-174" {
		t.Fatalf("staged Core = %q", got)
	}
	if got := readBackupTestFile(t, filepath.Join(staged, "web", "index.html")); got != "working-web-174" {
		t.Fatalf("staged Web = %q", got)
	}
	if got := readBackupTestFile(t, filepath.Join(staged, "helper", "home-ai-core-updater")); got != "working-helper-174" {
		t.Fatalf("staged helper = %q", got)
	}
	metadata, err := readBackupMetadata(staged)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Version != "0.1.174-dev" || metadata.ReplacedBy != "0.1.175-dev" {
		t.Fatalf("staged metadata = %+v", metadata)
	}
}

func TestStagingFailureKeepsPreviouslyCommittedRollback(t *testing.T) {
	root := t.TempDir()
	previous := filepath.Join(root, "backup")
	writeBackupTestFile(t, filepath.Join(previous, "bin", "home-ai-core"), "working-core-173")

	core := filepath.Join(root, "live", "core")
	writeBackupTestFile(t, core, "working-core-174")
	_, err := stageRollbackBackup(root, core, filepath.Join(root, "missing-web"), "", "0.1.174-dev", "0.1.175-dev", false)
	if err == nil {
		t.Fatal("staging missing Web unexpectedly succeeded")
	}
	if got := readBackupTestFile(t, filepath.Join(previous, "bin", "home-ai-core")); got != "working-core-173" {
		t.Fatalf("previous rollback was modified: %q", got)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".backup-candidate-") {
			t.Fatalf("incomplete rollback staging was left behind: %s", entry.Name())
		}
	}
}

func TestHelperCopyFailureDoesNotReplaceCommittedRollback(t *testing.T) {
	root := t.TempDir()
	previous := filepath.Join(root, "backup")
	writeBackupTestFile(t, filepath.Join(previous, "marker"), "previous")
	core := filepath.Join(root, "live", "core")
	web := filepath.Join(root, "live", "web")
	writeBackupTestFile(t, core, "core")
	writeBackupTestFile(t, filepath.Join(web, "index.html"), "web")

	_, err := stageRollbackBackup(root, core, web, filepath.Join(root, "missing-helper"), "0.1.174-dev", "0.1.175-dev", true)
	if err == nil {
		t.Fatal("staging without the required updater helper unexpectedly succeeded")
	}
	if got := readBackupTestFile(t, filepath.Join(previous, "marker")); got != "previous" {
		t.Fatalf("previous backup changed: %q", got)
	}
}

func TestPromotionExchangesSnapshotsWithoutDeletingPrevious(t *testing.T) {
	root := t.TempDir()
	previous := filepath.Join(root, "backup")
	staged := filepath.Join(root, ".backup-candidate-test")
	writeBackupTestFile(t, filepath.Join(previous, "marker"), "previous-173")
	writeBackupTestFile(t, filepath.Join(staged, "marker"), "new-rollback-174")

	if err := replaceDirectoryAtomically(staged, previous); err != nil {
		t.Fatal(err)
	}
	if got := readBackupTestFile(t, filepath.Join(previous, "marker")); got != "new-rollback-174" {
		t.Fatalf("active rollback = %q", got)
	}
	if got := readBackupTestFile(t, filepath.Join(staged, "marker")); got != "previous-173" {
		t.Fatalf("previous rollback disappeared: %q", got)
	}
}

func TestPromotionWithoutExistingBackup(t *testing.T) {
	root := t.TempDir()
	staged := filepath.Join(root, ".backup-candidate-test")
	active := filepath.Join(root, "backup")
	writeBackupTestFile(t, filepath.Join(staged, "marker"), "first-backup")

	if err := replaceDirectoryAtomically(staged, active); err != nil {
		t.Fatal(err)
	}
	if got := readBackupTestFile(t, filepath.Join(active, "marker")); got != "first-backup" {
		t.Fatalf("active rollback = %q", got)
	}
	if _, err := os.Lstat(staged); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("staged directory should have been moved, got %v", err)
	}
}

func TestPromotionRejectsUnsafeActivePath(t *testing.T) {
	root := t.TempDir()
	staged := filepath.Join(root, ".backup-candidate-test")
	victim := filepath.Join(root, "victim")
	active := filepath.Join(root, "backup")
	writeBackupTestFile(t, filepath.Join(staged, "marker"), "new")
	writeBackupTestFile(t, filepath.Join(victim, "marker"), "keep")
	if err := os.Symlink(victim, active); err != nil {
		t.Fatal(err)
	}
	if err := replaceDirectoryAtomically(staged, active); err == nil {
		t.Fatal("symlink destination unexpectedly accepted")
	}
	if got := readBackupTestFile(t, filepath.Join(victim, "marker")); got != "keep" {
		t.Fatalf("symlink destination was altered: %q", got)
	}
	if got := readBackupTestFile(t, filepath.Join(staged, "marker")); got != "new" {
		t.Fatalf("staged source was altered: %q", got)
	}
}

func TestRollbackStageRejectsUnknownSourceVersion(t *testing.T) {
	root := t.TempDir()
	_, err := stageRollbackBackup(root, "unneeded", "unneeded", "unneeded", "", "0.1.175-dev", false)
	if err == nil {
		t.Fatal("rollback with unknown running version unexpectedly accepted")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("invalid version created backup files: %v", entries)
	}
}

func TestUnresolvedUpdateStagingBlocksAnotherInstall(t *testing.T) {
	root := t.TempDir()
	rollbackWeb := filepath.Join(root, "web-rollback")
	if err := refuseInterruptedBackupStaging(root, rollbackWeb); err != nil {
		t.Fatalf("clean root rejected: %v", err)
	}
	pending := filepath.Join(root, ".backup-candidate-orphan")
	writeBackupTestFile(t, filepath.Join(pending, "marker"), "recover-me")
	if err := refuseInterruptedBackupStaging(root, rollbackWeb); err == nil {
		t.Fatal("interrupted rollback snapshot was not detected")
	}
	if got := readBackupTestFile(t, filepath.Join(pending, "marker")); got != "recover-me" {
		t.Fatalf("previous recovery snapshot altered: %q", got)
	}
	if err := os.RemoveAll(pending); err != nil {
		t.Fatal(err)
	}
	writeBackupTestFile(t, filepath.Join(rollbackWeb, "index.html"), "previous-web")
	if err := refuseInterruptedBackupStaging(root, rollbackWeb); err == nil {
		t.Fatal("interrupted Web UI backup was not detected")
	}
	if got := readBackupTestFile(t, filepath.Join(rollbackWeb, "index.html")); got != "previous-web" {
		t.Fatalf("previous Web UI backup altered: %q", got)
	}
}
