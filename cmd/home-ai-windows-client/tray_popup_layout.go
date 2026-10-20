package main

// Tray popup geometry uses logical 96-DPI units. Controls, font sizes,
// and the popup itself must all be scaled by the same effective DPI so that
// text does not overlap or extend past button boundaries on scaled monitors.
const (
	trayPopupLogicalWidth  int32 = 440
	trayPopupLogicalHeight int32 = 466
	trayPopupMargin        int32 = 8
)

type trayPopupRect struct {
	X, Y, Width, Height int32
}

type trayPopupLayout struct {
	Width, Height int32
	Icon          trayPopupRect
	Title         trayPopupRect
	Headline      trayPopupRect
	Subtitle      trayPopupRect
	RecentTitle   trayPopupRect
	Rows          [3]trayPopupRect
	SyncButton    trayPopupRect
	FolderButton  trayPopupRect
	SettingsButton trayPopupRect
}

func trayPopupScale(v int32, dpi uint32) int32 {
	if dpi == 0 {
		dpi = 96
	}
	if v < 0 {
		return -trayPopupScale(-v, dpi)
	}
	return int32((int64(v)*int64(dpi) + 48) / 96)
}

func trayPopupEffectiveDPI(dpi uint32, workWidth, workHeight int32) uint32 {
	if dpi < 72 || dpi > 768 {
		dpi = 96
	}
	if workWidth > 2*trayPopupMargin && workHeight > 2*trayPopupMargin {
		availableW := workWidth - 2*trayPopupMargin
		availableH := workHeight - 2*trayPopupMargin
		maxW := uint32(int64(availableW) * 96 / int64(trayPopupLogicalWidth))
		maxH := uint32(int64(availableH) * 96 / int64(trayPopupLogicalHeight))
		if dpi > maxW {
			dpi = maxW
		}
		if dpi > maxH {
			dpi = maxH
		}
		if dpi < 72 {
			dpi = 72
		}
	}
	return dpi
}

func trayPopupBuildLayout(dpi uint32) trayPopupLayout {
	s := func(x, y, w, h int32) trayPopupRect {
		return trayPopupRect{trayPopupScale(x, dpi), trayPopupScale(y, dpi), trayPopupScale(w, dpi), trayPopupScale(h, dpi)}
	}
	layout := trayPopupLayout{
		Width:          trayPopupScale(trayPopupLogicalWidth, dpi),
		Height:         trayPopupScale(trayPopupLogicalHeight, dpi),
		Icon:           s(20, 18, 34, 34),
		Title:          s(66, 20, 354, 34),
		Headline:       s(20, 73, 400, 32),
		Subtitle:       s(20, 113, 400, 42),
		RecentTitle:    s(20, 173, 400, 26),
		SyncButton:     s(20, 366, 190, 40),
		FolderButton:   s(220, 366, 200, 40),
		SettingsButton: s(220, 415, 200, 38),
	}
	for i := range layout.Rows {
		layout.Rows[i] = s(22, int32(205+i*50), 396, 46)
	}
	return layout
}

func trayPopupClamp(value, min, max int32) int32 {
	if max < min {
		return min
	}
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

// Windows tray icons are normally at the bottom/right of the current monitor,
// but monitors can extend into negative desktop coordinates. Place the popup
// wholly inside that monitor's available work area.
func trayPopupPosition(cursorX, cursorY, workLeft, workTop, workRight, workBottom, width, height int32) (int32, int32) {
	x := cursorX - width + 34
	y := cursorY - height - 14
	if workRight > workLeft && workBottom > workTop {
		x = trayPopupClamp(x, workLeft+trayPopupMargin, workRight-width-trayPopupMargin)
		y = trayPopupClamp(y, workTop+trayPopupMargin, workBottom-height-trayPopupMargin)
	}
	return x, y
}
