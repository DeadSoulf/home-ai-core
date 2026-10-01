//go:build windows

package main

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	settingsWMPaint          = 0x000F
	settingsWMCTLColorStatic = 0x0138
	settingsTransparent      = 1

	settingsFWNormal   = 400
	settingsFWSemibold = 600
	settingsFWBold     = 700

	settingsVisualMain settingsVisualRole = iota
	settingsVisualSidebar
	settingsVisualHero
	settingsVisualCard
	settingsVisualMuted
	settingsVisualPositive
	settingsVisualStatusIcon
)

type settingsVisualRole int

type settingsRect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

type settingsPaintStruct struct {
	HDC       windows.Handle
	Erase     int32
	Paint     settingsRect
	Restore   int32
	IncUpdate int32
	Reserved  [32]byte
}

type settingsVisualResources struct {
	mainBrush     windows.Handle
	sidebarBrush  windows.Handle
	heroBrush     windows.Handle
	cardBrush     windows.Handle
	successBrush  windows.Handle
	statusBrush   windows.Handle
	accentBrush   windows.Handle
	borderPen     windows.Handle
	titleFont     windows.Handle
	headlineFont  windows.Handle
	subtitleFont  windows.Handle
	cardTitleFont windows.Handle
	cardValueFont windows.Handle
}

var (
	procSettingsBeginPaint     = settingsUser32.NewProc("BeginPaint")
	procSettingsEndPaint       = settingsUser32.NewProc("EndPaint")
	procSettingsGetClientRect  = settingsUser32.NewProc("GetClientRect")
	procSettingsFillRect       = settingsUser32.NewProc("FillRect")
	procSettingsInvalidateRect = settingsUser32.NewProc("InvalidateRect")
	procSettingsSetTextColor   = settingsGDI32.NewProc("SetTextColor")
	procSettingsSetBkColor     = settingsGDI32.NewProc("SetBkColor")
	procSettingsSetBkMode      = settingsGDI32.NewProc("SetBkMode")
	procSettingsCreateBrush    = settingsGDI32.NewProc("CreateSolidBrush")
	procSettingsCreatePen      = settingsGDI32.NewProc("CreatePen")
	procSettingsSelectObject   = settingsGDI32.NewProc("SelectObject")
	procSettingsDeleteObject   = settingsGDI32.NewProc("DeleteObject")
	procSettingsRoundRect      = settingsGDI32.NewProc("RoundRect")
	procSettingsEllipse        = settingsGDI32.NewProc("Ellipse")
	procSettingsCreateFont     = settingsGDI32.NewProc("CreateFontW")
)

func settingsRGB(r, g, b byte) uintptr {
	return uintptr(uint32(r) | uint32(g)<<8 | uint32(b)<<16)
}

func createSettingsFont(height int32, weight int32) windows.Handle {
	face, _ := windows.UTF16PtrFromString("Segoe UI")
	h, _, _ := procSettingsCreateFont.Call(
		uintptr(height),
		0, 0, 0,
		uintptr(weight),
		0, 0, 0,
		1,
		0, 0, 5, 0,
		uintptr(unsafe.Pointer(face)),
	)
	return windows.Handle(h)
}

func (state *windowsSettingsUI) initVisualResources() error {
	state.visual.mainBrush = windows.Handle(mustCreateBrush(248, 250, 252))
	state.visual.sidebarBrush = windows.Handle(mustCreateBrush(246, 249, 253))
	state.visual.heroBrush = windows.Handle(mustCreateBrush(232, 244, 255))
	state.visual.cardBrush = windows.Handle(mustCreateBrush(255, 255, 255))
	state.visual.successBrush = windows.Handle(mustCreateBrush(230, 247, 238))
	state.visual.statusBrush = windows.Handle(mustCreateBrush(47, 184, 113))
	state.visual.accentBrush = windows.Handle(mustCreateBrush(36, 111, 220))
	pen, _, _ := procSettingsCreatePen.Call(0, 1, settingsRGB(224, 230, 238))
	state.visual.borderPen = windows.Handle(pen)
	state.visual.titleFont = createSettingsFont(-22, settingsFWSemibold)
	state.visual.headlineFont = createSettingsFont(-30, settingsFWBold)
	state.visual.subtitleFont = createSettingsFont(-16, settingsFWNormal)
	state.visual.cardTitleFont = createSettingsFont(-14, settingsFWSemibold)
	state.visual.cardValueFont = createSettingsFont(-17, settingsFWBold)
	if state.visual.mainBrush == 0 || state.visual.sidebarBrush == 0 ||
		state.visual.heroBrush == 0 || state.visual.cardBrush == 0 ||
		state.visual.successBrush == 0 || state.visual.statusBrush == 0 ||
		state.visual.accentBrush == 0 || state.visual.borderPen == 0 || state.visual.titleFont == 0 ||
		state.visual.headlineFont == 0 || state.visual.subtitleFont == 0 ||
		state.visual.cardTitleFont == 0 || state.visual.cardValueFont == 0 {
		return fmt.Errorf("create Windows client visual resources")
	}
	if state.visualRoles == nil {
		state.visualRoles = make(map[windows.Handle]settingsVisualRole)
	}
	return nil
}

func mustCreateBrush(r, g, b byte) uintptr {
	handle, _, _ := procSettingsCreateBrush.Call(settingsRGB(r, g, b))
	return handle
}

func (state *windowsSettingsUI) releaseVisualResources() {
	for _, handle := range []windows.Handle{
		state.visual.mainBrush,
		state.visual.sidebarBrush,
		state.visual.heroBrush,
		state.visual.cardBrush,
		state.visual.successBrush,
		state.visual.statusBrush,
		state.visual.accentBrush,
		state.visual.borderPen,
		state.visual.titleFont,
		state.visual.headlineFont,
		state.visual.subtitleFont,
		state.visual.cardTitleFont,
		state.visual.cardValueFont,
	} {
		if handle != 0 {
			procSettingsDeleteObject.Call(uintptr(handle))
		}
	}
	state.visual = settingsVisualResources{}
}

func (state *windowsSettingsUI) setVisualRole(hwnd windows.Handle, role settingsVisualRole) {
	if hwnd == 0 {
		return
	}
	if state.visualRoles == nil {
		state.visualRoles = make(map[windows.Handle]settingsVisualRole)
	}
	state.visualRoles[hwnd] = role
}

func (state *windowsSettingsUI) setControlFont(hwnd, font windows.Handle) {
	if hwnd == 0 || font == 0 {
		return
	}
	procSettingsSendMessage.Call(uintptr(hwnd), settingsWMSetFont, uintptr(font), 1)
}

func (state *windowsSettingsUI) visualBrush(role settingsVisualRole) windows.Handle {
	switch role {
	case settingsVisualSidebar:
		return state.visual.sidebarBrush
	case settingsVisualHero:
		return state.visual.heroBrush
	case settingsVisualCard:
		return state.visual.cardBrush
	case settingsVisualPositive:
		return state.visual.successBrush
	case settingsVisualStatusIcon:
		return state.visual.heroBrush
	default:
		return state.visual.mainBrush
	}
}

func (state *windowsSettingsUI) handleStaticColor(hdc, hwnd uintptr) uintptr {
	role := state.visualRoles[windows.Handle(hwnd)]
	brush := state.visualBrush(role)
	bg := settingsRGB(248, 250, 252)
	text := settingsRGB(29, 41, 57)
	switch role {
	case settingsVisualSidebar:
		bg = settingsRGB(246, 249, 253)
	case settingsVisualHero:
		bg = settingsRGB(232, 244, 255)
	case settingsVisualCard:
		bg = settingsRGB(255, 255, 255)
	case settingsVisualMuted:
		text = settingsRGB(103, 116, 137)
	case settingsVisualPositive:
		bg = settingsRGB(230, 247, 238)
		text = settingsRGB(23, 122, 72)
	case settingsVisualStatusIcon:
		bg = settingsRGB(232, 244, 255)
		text = settingsRGB(255, 255, 255)
	}
	procSettingsSetTextColor.Call(hdc, text)
	procSettingsSetBkColor.Call(hdc, bg)
	procSettingsSetBkMode.Call(hdc, settingsTransparent)
	return uintptr(brush)
}

func (state *windowsSettingsUI) paintWindow(hwnd uintptr) uintptr {
	var ps settingsPaintStruct
	hdc, _, _ := procSettingsBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	if hdc == 0 {
		return 0
	}
	defer procSettingsEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))

	var client settingsRect
	procSettingsGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&client)))
	procSettingsFillRect.Call(hdc, uintptr(unsafe.Pointer(&client)), uintptr(state.visual.mainBrush))

	sidebar := settingsRect{Left: 0, Top: 0, Right: 205, Bottom: client.Bottom}
	procSettingsFillRect.Call(hdc, uintptr(unsafe.Pointer(&sidebar)), uintptr(state.visual.sidebarBrush))

	if state.page == settingsPageOverview {
		state.paintOverviewCards(windows.Handle(hdc))
	} else {
		state.paintRoundedPanel(windows.Handle(hdc), 220, 18, client.Right-22, client.Bottom-34, state.visual.cardBrush, 18)
	}
	return 0
}

func (state *windowsSettingsUI) paintOverviewCards(hdc windows.Handle) {
	state.paintRoundedPanel(hdc, 225, 64, 1150, 188, state.visual.heroBrush, 20)
	state.paintStatusCircle(hdc, 246, 91, 292, 137)

	for _, rect := range [][4]int32{
		{225, 214, 442, 312},
		{456, 214, 673, 312},
		{687, 214, 904, 312},
		{918, 214, 1150, 312},
		{225, 334, 795, 594},
		{815, 334, 1150, 452},
		{815, 474, 1150, 594},
	} {
		state.paintRoundedPanel(hdc, rect[0], rect[1], rect[2], rect[3], state.visual.cardBrush, 16)
	}

	track := settingsRect{Left: 842, Top: 552, Right: 1122, Bottom: 562}
	procSettingsFillRect.Call(uintptr(hdc), uintptr(unsafe.Pointer(&track)), uintptr(state.visual.heroBrush))
	percent := state.overviewStoragePercent
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	if percent > 0 {
		fill := track
		fill.Right = fill.Left + int32((int64(track.Right-track.Left)*int64(percent))/100)
		procSettingsFillRect.Call(uintptr(hdc), uintptr(unsafe.Pointer(&fill)), uintptr(state.visual.accentBrush))
	}
}

func (state *windowsSettingsUI) paintStatusCircle(hdc windows.Handle, left, top, right, bottom int32) {
	oldBrush, _, _ := procSettingsSelectObject.Call(uintptr(hdc), uintptr(state.visual.statusBrush))
	nullPen, _, _ := procSettingsGetStock.Call(8)
	oldPen, _, _ := procSettingsSelectObject.Call(uintptr(hdc), nullPen)
	procSettingsEllipse.Call(uintptr(hdc), uintptr(left), uintptr(top), uintptr(right), uintptr(bottom))
	if oldBrush != 0 {
		procSettingsSelectObject.Call(uintptr(hdc), oldBrush)
	}
	if oldPen != 0 {
		procSettingsSelectObject.Call(uintptr(hdc), oldPen)
	}
}

func (state *windowsSettingsUI) paintRoundedPanel(hdc windows.Handle, left, top, right, bottom int32, brush windows.Handle, radius int32) {
	oldBrush, _, _ := procSettingsSelectObject.Call(uintptr(hdc), uintptr(brush))
	oldPen, _, _ := procSettingsSelectObject.Call(uintptr(hdc), uintptr(state.visual.borderPen))
	procSettingsRoundRect.Call(uintptr(hdc), uintptr(left), uintptr(top), uintptr(right), uintptr(bottom), uintptr(radius), uintptr(radius))
	if oldBrush != 0 {
		procSettingsSelectObject.Call(uintptr(hdc), oldBrush)
	}
	if oldPen != 0 {
		procSettingsSelectObject.Call(uintptr(hdc), oldPen)
	}
}

func (state *windowsSettingsUI) invalidateVisual() {
	if state.hwnd != 0 {
		procSettingsInvalidateRect.Call(uintptr(state.hwnd), 0, 1)
	}
}
