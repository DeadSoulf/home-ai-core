package main

import "testing"

func TestTrayPopupLayoutDoesNotClipControls(t *testing.T) {
	for _, dpi := range []uint32{72, 96, 120, 144, 168, 192, 240, 288, 384} {
		t.Run(string(rune(dpi)), func(t *testing.T) {
			l := trayPopupBuildLayout(dpi)
			items := []trayPopupRect{
				l.Icon, l.Title, l.Headline, l.Subtitle, l.RecentTitle,
				l.Rows[0], l.Rows[1], l.Rows[2],
				l.SyncButton, l.FolderButton, l.SettingsButton,
			}
			for _, r := range items {
				if r.X < 0 || r.Y < 0 || r.Width <= 0 || r.Height <= 0 ||
					r.X+r.Width > l.Width || r.Y+r.Height > l.Height {
					t.Errorf("DPI %d: popup rect %#v outside %dx%d", dpi, r, l.Width, l.Height)
				}
			}
			// Every activity row must end before the action buttons; this
			// was the original clipping/overlap failure at increased DPI.
			if l.Rows[2].Y+l.Rows[2].Height >= l.SyncButton.Y ||
				l.SyncButton.Y+l.SyncButton.Height >= l.SettingsButton.Y {
				t.Errorf("DPI %d: status rows and buttons overlap: %+v", dpi, l)
			}
			if l.SyncButton.X+l.SyncButton.Width >= l.FolderButton.X {
				t.Errorf("DPI %d: horizontal action buttons overlap", dpi)
			}
		})
	}
}

func TestTrayPopupDPIIsBoundedByWorkArea(t *testing.T) {
	for _, test := range []struct {
		dpi  uint32
		workWidth, workHeight int32
	}{
		{96, 1920, 1040},
		{144, 1920, 1040},
		{192, 1920, 1040},
		{288, 1920, 1040},
		{384, 1366, 730},
		{192, 1600, 860},
		{240, 1080, 1760},
	} {
		dpi := trayPopupEffectiveDPI(test.dpi, test.workWidth, test.workHeight)
		l := trayPopupBuildLayout(dpi)
		if l.Width > test.workWidth-2*trayPopupMargin || l.Height > test.workHeight-2*trayPopupMargin {
			t.Errorf("DPI %d (%dx%d monitor): popup %dx%d does not fit at effective DPI %d",
				test.dpi, test.workWidth, test.workHeight, l.Width, l.Height, dpi)
		}
	}
}

func TestTrayPopupPositionWithinMonitorEvenWithNegativeCoordinates(t *testing.T) {
	for _, test := range []struct {
		cursorX, cursorY int32
		left, top, right, bottom int32
	}{
		{1900, 1050, 0, 0, 1920, 1080},
		{-50, 850, -1600, 0, 0, 900},
		{45, 15, 0, 0, 1920, 1080},
		{2000, 100, 1920, 0, 3840, 1080},
	} {
		x, y := trayPopupPosition(test.cursorX, test.cursorY,
			test.left, test.top, test.right, test.bottom, 780, 810)
		if x < test.left+trayPopupMargin || x+780 > test.right-trayPopupMargin ||
			y < test.top+trayPopupMargin || y+810 > test.bottom-trayPopupMargin {
			t.Errorf("position (%d,%d) leaves work area %+v", x, y, test)
		}
	}
}
