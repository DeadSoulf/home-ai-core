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
	if state.visual.mainBrush == 0 || state.visual.cardBrush == 0 ||
		state.visual.navBorderPen == 0 || state.visual.brandFont == 0 ||
		state.visual.headlineFont == 0 {
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

func TestCompactFigmaGeometry(t *testing.T) {
	if settingsCompactWindowWidth != 976 || settingsCompactWindowHeight != 635 {
		t.Fatalf("compact window = %dx%d, want 976x635", settingsCompactWindowWidth, settingsCompactWindowHeight)
	}
	if settingsSidebarWidth != 196 {
		t.Fatalf("sidebar width = %d, want 196", settingsSidebarWidth)
	}
}

func TestDashboardFolderCount(t *testing.T) {
	tests := []struct {
		count    int
		language string
		want     string
	}{
		{1, uiLanguageRussian, "1 папка"},
		{2, uiLanguageRussian, "2 папки"},
		{5, uiLanguageRussian, "5 папок"},
		{11, uiLanguageRussian, "11 папок"},
		{1, uiLanguageEnglish, "1 folder"},
		{3, uiLanguageEnglish, "3 folders"},
	}
	for _, tt := range tests {
		if got := dashboardFolderCount(tt.count, tt.language); got != tt.want {
			t.Fatalf("dashboardFolderCount(%d, %q) = %q, want %q", tt.count, tt.language, got, tt.want)
		}
	}
}

func TestSettingsButtonRoles(t *testing.T) {
	tests := map[string]settingsButtonRole{
		"overview_sync_button": settingsButtonHeroPrimary,
		"overview_open_button": settingsButtonHeroSecondary,
		"connect_button":       settingsButtonPrimary,
		"browse_button":        settingsButtonSecondary,
	}
	for name, want := range tests {
		if got := settingsButtonRoleForName(name); got != want {
			t.Fatalf("settingsButtonRoleForName(%q) = %d, want %d", name, got, want)
		}
	}
	if settingsButtonCornerRadius != 10 {
		t.Fatalf("settingsButtonCornerRadius = %d, want 10", settingsButtonCornerRadius)
	}
}
