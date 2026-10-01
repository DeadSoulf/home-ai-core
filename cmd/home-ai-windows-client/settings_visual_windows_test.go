//go:build windows

package main

import (
	"path/filepath"
	"testing"
)

func TestSettingsVisualResources(t *testing.T) {
	state := &windowsSettingsUI{}
	if err := state.initVisualResources(); err != nil {
		t.Fatalf("init visual resources: %v", err)
	}
	defer state.releaseVisualResources()
	if state.visual.mainBrush == 0 || state.visual.cardBrush == 0 || state.visual.headlineFont == 0 {
		t.Fatal("visual resources were not created")
	}
}

func TestDashboardBytes(t *testing.T) {
	tests := map[int64]string{
		0:           "0 B",
		1024:        "1.0 KiB",
		1024 * 1024: "1.0 MiB",
	}
	for value, want := range tests {
		if got := dashboardBytes(value); got != want {
			t.Fatalf("dashboardBytes(%d) = %q, want %q", value, got, want)
		}
	}
}

func TestTrayPopupSnapshotWithoutProfiles(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "sync.json")
	headline, subtitle, rows := trayPopupSnapshot(configPath, uiLanguageEnglish)
	if headline == "" || subtitle == "" {
		t.Fatal("empty tray popup status")
	}
	for i, row := range rows {
		if row == "" {
			t.Fatalf("tray popup row %d is empty", i)
		}
	}
}
