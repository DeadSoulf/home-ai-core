//go:build windows

package main

import "testing"

func TestSettingsDPIScale(t *testing.T) {
	for _, sample := range []struct {
		value int32
		dpi   uint32
		want  int32
	}{
		{96, 96, 96},
		{96, 120, 120},
		{96, 144, 144},
		{96, 192, 192},
		{-20, 144, -30},
	} {
		if got := settingsScale(sample.value, sample.dpi); got != sample.want {
			t.Fatalf("settingsScale(%d,%d)=%d, want %d", sample.value, sample.dpi, got, sample.want)
		}
	}
}

func TestSettingsReflowAnchors(t *testing.T) {
	tests := []struct {
		name   string
		base   settingsLayoutRect
		page   int
		dx, dy int32
		want   settingsLayoutRect
	}{
		{"sidebar stays fixed", settingsLayoutRect{16, 112, 164, 36}, -1, 320, 220, settingsLayoutRect{16, 112, 164, 36}},
		{"list grows with client", settingsLayoutRect{224, 398, 680, 120}, settingsPageSync, 320, 220, settingsLayoutRect{224, 398, 1000, 340}},
		{"sync action anchored right", settingsLayoutRect{704, 532, 200, 34}, settingsPageSync, 320, 220, settingsLayoutRect{1024, 752, 200, 34}},
		{"transfer history grows", settingsLayoutRect{224, 288, 680, 226}, settingsPageTransfers, 320, 220, settingsLayoutRect{224, 288, 1000, 446}},
		{"status pinned bottom", settingsLayoutRect{302, 560, 602, 20}, -1, 320, 220, settingsLayoutRect{302, 780, 922, 20}},
	}
	for _, tt := range tests {
		if got := settingsReflowRect(tt.base, tt.page, tt.dx, tt.dy); got != tt.want {
			t.Errorf("%s: got %+v, want %+v", tt.name, got, tt.want)
		}
	}
}
