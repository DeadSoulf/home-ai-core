//go:build windows

package main

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	settingsWMSize          = 0x0005
	settingsWMGetMinMaxInfo = 0x0024
	settingsWMDPIChanged    = 0x02E0
	settingsSWPNoZOrder     = 0x0004
	settingsSWPNoActivate   = 0x0010

	// The 976x635 Figma window contains approximately a 960x596 client.
	// The original coordinates remain 96-DPI design units, not device pixels.
	settingsDesignClientWidth  = 960
	settingsDesignClientHeight = 596
)

type settingsLayoutRect struct {
	x, y, w, h int32
}

type settingsMinMaxInfo struct {
	reserved     settingsPoint
	maxSize      settingsPoint
	maxPosition  settingsPoint
	minTrackSize settingsPoint
	maxTrackSize settingsPoint
}

var (
	procSettingsSetProcessDPI = settingsUser32.NewProc("SetProcessDpiAwarenessContext")
	procSettingsGetDPIForSystem = settingsUser32.NewProc("GetDpiForSystem")
	procSettingsGetDPIForWindow = settingsUser32.NewProc("GetDpiForWindow")
	procSettingsMoveWindow = settingsUser32.NewProc("MoveWindow")
	procSettingsSetWindowPos = settingsUser32.NewProc("SetWindowPos")
)

// Manifest PerMonitorV2 takes precedence. An explicit call also covers local
// development executables built without embedded Windows resources.
func settingsEnableDPIAwareness() {
	if procSettingsSetProcessDPI.Find() == nil {
		procSettingsSetProcessDPI.Call(^uintptr(3)) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 (-4)
	}
}

func settingsSystemDPI() uint32 {
	if procSettingsGetDPIForSystem.Find() == nil {
		dpi, _, _ := procSettingsGetDPIForSystem.Call()
		if dpi >= 96 && dpi <= 768 {
			return uint32(dpi)
		}
	}
	return 96
}

func settingsWindowDPI(hwnd windows.Handle) uint32 {
	if hwnd != 0 && procSettingsGetDPIForWindow.Find() == nil {
		dpi, _, _ := procSettingsGetDPIForWindow.Call(uintptr(hwnd))
		if dpi >= 96 && dpi <= 768 {
			return uint32(dpi)
		}
	}
	return settingsSystemDPI()
}

func settingsScale(value int32, dpi uint32) int32 {
	if dpi == 0 {
		dpi = 96
	}
	if value < 0 {
		return -settingsScale(-value, dpi)
	}
	return int32((int64(value)*int64(dpi) + 48) / 96)
}

func (state *windowsSettingsUI) scale(value int32) int32 {
	return settingsScale(value, state.dpi)
}

func (state *windowsSettingsUI) layoutExtra() (int32, int32) {
	if state.hwnd == 0 {
		return 0, 0
	}
	var rect settingsRect
	procSettingsGetClientRect.Call(uintptr(state.hwnd), uintptr(unsafe.Pointer(&rect)))
	dpi := state.dpi
	if dpi == 0 {
		dpi = 96
	}
	toLogical := func(v int32) int32 { return int32(int64(v)*96/int64(dpi)) }
	dx := toLogical(rect.Right) - settingsDesignClientWidth
	dy := toLogical(rect.Bottom) - settingsDesignClientHeight
	if dx < 0 {
		dx = 0
	}
	if dy < 0 {
		dy = 0
	}
	return dx, dy
}

func (state *windowsSettingsUI) controlPage(hwnd windows.Handle) int {
	for page := range state.pageControls {
		for _, control := range state.pageControls[page] {
			if control == hwnd {
				return page
			}
		}
	}
	return -1
}

// Geometry rules operate in logical 96-DPI units. The sidebar is fixed;
// forms stretch, secondary actions stay right-aligned, and transfer/history
// lists grow vertically. Unlike proportional stretching, fonts do not distort.
func settingsReflowRect(r settingsLayoutRect, page int, dx, dy int32) settingsLayoutRect {
	if r.x < settingsSidebarWidth {
		if r.y >= 540 {
			r.y += dy
		}
		return r
	}

	switch page {
	case settingsPageOverview:
		if r.y >= 220 && r.y < 312 {
			slot := (r.x - 224) / 177
			if slot < 0 {
				slot = 0
			}
			if slot > 3 {
				slot = 3
			}
			r.x += dx * slot / 4
			r.w += dx / 4
		} else if r.y >= 332 {
			if r.x >= 696 {
				r.x += dx
			} else if r.w >= 300 {
				r.w += dx
			}
			if r.y >= 535 {
				r.y += dy
			}
		} else if r.y >= 88 && r.x >= 700 {
			r.x += dx
		} else if r.y >= 88 && r.w >= 350 {
			r.w += dx
		}
	case settingsPageSync:
		if r.y == 326 && r.x >= 374 {
			switch {
			case r.x >= 818:
				r.x += dx
			case r.x >= 722:
				r.x += dx * 4 / 5
			case r.x >= 616:
				r.x += dx * 3 / 5
			case r.x >= 510:
				r.x += dx * 2 / 5
			default:
				r.x += dx / 5
			}
		} else if r.y == 398 && r.w >= 600 {
			r.w += dx
			r.h += dy
		} else if r.y >= 532 && r.x >= 650 {
			r.x += dx
			r.y += dy
		} else if r.x >= 800 {
			r.x += dx
		} else if r.w >= 350 {
			r.w += dx
		}
	case settingsPageTransfers:
		if r.y >= 270 && r.h >= 100 {
			r.h += dy
		}
		if r.y >= 520 && r.h < 60 {
			r.y += dy
			if r.x >= 600 {
				r.x += dx
			}
		}
		if r.w >= 350 {
			r.w += dx
		}
	default:
		if page == settingsPageConnection && r.x >= 500 && r.y >= 198 && r.y <= 270 {
			r.x += dx / 2
			if r.w >= 200 {
				r.w += dx / 2
			}
		} else if page == settingsPageConnection && r.x == 224 && r.y >= 198 && r.y <= 270 && r.w < 300 {
			r.w += dx / 2
		} else if page == settingsPageBackup && r.x >= 476 && r.y >= 198 && r.y <= 260 {
			r.x += dx / 2
			if r.w >= 200 {
				r.w += dx / 2
			}
		} else if page == settingsPageBackup && r.x == 224 && r.y >= 198 && r.y <= 260 && r.w <= 240 {
			r.w += dx / 2
		} else if r.w >= 350 {
			r.w += dx
		}
	}
	if r.y >= 558 && page == -1 {
		r.y += dy // Global status row, pinned to the bottom.
		if r.x >= 300 {
			r.w += dx
		}
	}
	return r
}

func (state *windowsSettingsUI) layoutControls() {
	if state.hwnd == 0 || len(state.controlRects) == 0 {
		return
	}
	dx, dy := state.layoutExtra()
	for hwnd, base := range state.controlRects {
		r := settingsReflowRect(base, state.controlPage(hwnd), dx, dy)
		procSettingsMoveWindow.Call(uintptr(hwnd),
			uintptr(state.scale(r.x)), uintptr(state.scale(r.y)),
			uintptr(state.scale(r.w)), uintptr(state.scale(r.h)), 1)
		if _, isButton := state.buttonRoles[hwnd]; isButton {
			state.applyRoundedButtonRegion(hwnd, r.w, r.h)
		}
	}
	state.invalidateVisual()
}

func (state *windowsSettingsUI) handleDPIChange(hwnd, wParam, lParam uintptr) {
	dpi := uint32(wParam & 0xffff)
	if dpi < 96 || dpi > 768 || dpi == state.dpi {
		return
	}
	state.dpi = dpi
	state.rebuildDpiFonts()
	if lParam != 0 {
		suggested := (*settingsRect)(unsafe.Pointer(lParam))
		procSettingsSetWindowPos.Call(hwnd, 0,
			uintptr(suggested.Left), uintptr(suggested.Top),
			uintptr(suggested.Right-suggested.Left),
			uintptr(suggested.Bottom-suggested.Top),
			settingsSWPNoZOrder|settingsSWPNoActivate)
	}
	state.layoutControls()
}
