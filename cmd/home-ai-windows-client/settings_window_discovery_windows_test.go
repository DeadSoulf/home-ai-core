//go:build windows

package main

import "testing"

func TestRecognizeSettingsWindowClass(t *testing.T) {
	for _, item := range []struct {
		name string
		want bool
	}{
		{"HomeAIWindowsSettingsWindow", true},
		{"homeaiwindowssettingswindow", true},
		{"HomeAIWindowsSyncTray", false},
		{"HomeAIWindowsTrayPopup", false},
		{"", false},
	} {
		if got := isHomeAISettingsClass(item.name); got != item.want {
			t.Fatalf("class name %q => %v, want %v", item.name, got, item.want)
		}
	}
}
