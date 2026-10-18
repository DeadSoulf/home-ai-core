package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestAIDurableJournalRejectsOverwriteAndInvalidContents(t *testing.T) {
	previous := moduleDataRoot
	moduleDataRoot = t.TempDir()
	t.Cleanup(func() { moduleDataRoot = previous })
	state := aiUpgradeJournal{
		PreviousImage:    "ghcr.io/deadsoulf/home-ai-core-ai@sha256:" + strings.Repeat("a", 64),
		NextImage:        "ghcr.io/deadsoulf/home-ai-core-ai@sha256:" + strings.Repeat("b", 64),
		PreviousManifest: `{"id":"ai.agent","version":"0.3.0"}`,
	}
	if err := writeAIUpgradeJournal(state); err != nil {
		t.Fatal(err)
	}
	if err := writeAIUpgradeJournal(state); err == nil {
		t.Fatal("overwrote a pending upgrade recovery record")
	}
	got, pending, err := readAIUpgradeJournal()
	if err != nil || !pending || got.PreviousImage != state.PreviousImage || got.PreviousManifest != state.PreviousManifest {
		t.Fatalf("journal persistence: %+v, %t, %v", got, pending, err)
	}
	info, err := os.Stat(aiUpgradePath(aiUpgradeJournalName))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("journal must be private: %v %v", info, err)
	}
}

func TestAISnapshotRestoresDataAfterFailedCandidate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires GNU cp and POSIX Docker stub")
	}
	previous := moduleDataRoot
	moduleDataRoot = t.TempDir()
	t.Cleanup(func() { moduleDataRoot = previous })
	data := filepath.Join(moduleDataRoot, "ai.agent")
	if err := os.Mkdir(data, 0o700); err != nil {
		t.Fatal(err)
	}
	sqlite := filepath.Join(data, "ai.sqlite3")
	if err := os.WriteFile(sqlite, []byte("DIALOGUE AND CREDENTIALS"), 0o600); err != nil {
		t.Fatal(err)
	}
	state := aiUpgradeJournal{
		PreviousImage:    "ghcr.io/deadsoulf/home-ai-core-ai@sha256:" + strings.Repeat("a", 64),
		NextImage:        "ghcr.io/deadsoulf/home-ai-core-ai@sha256:" + strings.Repeat("b", 64),
		PreviousManifest: `{"id":"ai.agent","version":"0.3.0"}`,
	}
	if err := writeAIUpgradeJournal(state); err != nil {
		t.Fatal(err)
	}
	if err := aiUpgradeSnapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sqlite, []byte("BROKEN NEW MIGRATION"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "docker")
	script := "#!/bin/sh\n" +
		"case \"$1\" in\n" +
		" inspect)\n" +
		"   case \"$*\" in\n" +
		"     *ai.agent-upgrade-candidate*|*ai.agent-upgrade-previous*) echo 'No such object' >&2; exit 1;;\n" +
		"     *Health*) echo 'running|healthy';;\n" +
		"     *Labels*) echo 'true|ai.agent';;\n" +
		"     *Config.Image*) echo '" + state.PreviousImage + "';;\n" +
		"     *) echo 'sha256:" + strings.Repeat("c", 64) + "';;\n" +
		"   esac;;\n" +
		" start) exit 0;;\n" +
		" *) echo 'unexpected Docker operation' >&2; exit 1;;\n" +
		"esac\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rollbackAIUpgrade(ctx, path, state); err != nil {
		t.Fatal(err)
	}
	bytes, err := os.ReadFile(sqlite)
	if err != nil || string(bytes) != "DIALOGUE AND CREDENTIALS" {
		t.Fatalf("data restore failed: %q, %v", bytes, err)
	}
	if _, pending, err := readAIUpgradeJournal(); err != nil || pending {
		t.Fatalf("recovery journal not cleared after successful restore: %t %v", pending, err)
	}
}

func TestAIIncompleteSnapshotIsNotUsedForRestoration(t *testing.T) {
	old := moduleDataRoot
	moduleDataRoot = t.TempDir()
	t.Cleanup(func() { moduleDataRoot = old })
	data := filepath.Join(moduleDataRoot, "ai.agent")
	if err := os.Mkdir(data, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(aiUpgradePath(aiUpgradeBackupName), 0o700); err != nil {
		t.Fatal(err)
	}
	ready, err := aiUpgradeSnapshotReady()
	if err != nil || ready {
		t.Fatalf("partial backup accepted: %t, %v", ready, err)
	}
}
