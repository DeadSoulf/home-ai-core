//go:build windows

package main

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	settingsWMPaint          = 0x000F
	settingsWMDrawItem       = 0x002B
	settingsWMGetFont        = 0x0031
	settingsWMCTLColorStatic = 0x0138
	settingsTransparent      = 1

	settingsBSOwnerDraw = 0x0000000B

	settingsODSSelected = 0x0001
	settingsODSDisabled = 0x0004
	settingsODSFocus    = 0x0010

	settingsDTCenter      = 0x0001
	settingsDTVCenter     = 0x0004
	settingsDTSingleLine  = 0x0020
	settingsDTEndEllipsis = 0x8000

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

type settingsDrawItemStruct struct {
	CtlType    uint32
	CtlID      uint32
	ItemID     uint32
	ItemAction uint32
	ItemState  uint32
	HwndItem   windows.Handle
	HDC        windows.Handle
	Rect       settingsRect
	ItemData   uintptr
}

type settingsButtonRole int

const (
	settingsButtonSecondary settingsButtonRole = iota
	settingsButtonPrimary
	settingsButtonNavigation
)

type settingsPaintStruct struct {
	HDC       windows.Handle
	Erase     int32
	Paint     settingsRect
	Restore   int32
	IncUpdate int32
	Reserved  [32]byte
}

type settingsVisualResources struct {
	mainBrush          windows.Handle
	sidebarBrush       windows.Handle
	heroBrush          windows.Handle
	cardBrush          windows.Handle
	successBrush       windows.Handle
	statusBrush        windows.Handle
	accentBrush        windows.Handle
	accentPressedBrush windows.Handle
	borderPen          windows.Handle
	checkPen           windows.Handle
	titleFont          windows.Handle
	headlineFont       windows.Handle
	subtitleFont       windows.Handle
	cardTitleFont      windows.Handle
	cardValueFont      windows.Handle
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
	procSettingsMoveToEx       = settingsGDI32.NewProc("MoveToEx")
	procSettingsLineTo         = settingsGDI32.NewProc("LineTo")
	procSettingsDrawText       = settingsUser32.NewProc("DrawTextW")
	procSettingsDrawFocusRect  = settingsUser32.NewProc("DrawFocusRect")
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
	state.visual.accentPressedBrush = windows.Handle(mustCreateBrush(27, 91, 185))
	pen, _, _ := procSettingsCreatePen.Call(0, 1, settingsRGB(224, 230, 238))
	state.visual.borderPen = windows.Handle(pen)
	checkPen, _, _ := procSettingsCreatePen.Call(0, 4, settingsRGB(255, 255, 255))
	state.visual.checkPen = windows.Handle(checkPen)
	state.visual.titleFont = createSettingsFont(-20, settingsFWSemibold)
	state.visual.headlineFont = createSettingsFont(-26, settingsFWBold)
	state.visual.subtitleFont = createSettingsFont(-14, settingsFWNormal)
	state.visual.cardTitleFont = createSettingsFont(-13, settingsFWSemibold)
	state.visual.cardValueFont = createSettingsFont(-15, settingsFWSemibold)
	if state.visual.mainBrush == 0 || state.visual.sidebarBrush == 0 ||
		state.visual.heroBrush == 0 || state.visual.cardBrush == 0 ||
		state.visual.successBrush == 0 || state.visual.statusBrush == 0 ||
		state.visual.accentBrush == 0 || state.visual.accentPressedBrush == 0 ||
		state.visual.borderPen == 0 || state.visual.checkPen == 0 || state.visual.titleFont == 0 ||
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
		state.visual.accentPressedBrush,
		state.visual.borderPen,
		state.visual.checkPen,
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

	oldCheckPen, _, _ := procSettingsSelectObject.Call(uintptr(hdc), uintptr(state.visual.checkPen))
	procSettingsMoveToEx.Call(uintptr(hdc), uintptr(left+12), uintptr(top+24), 0)
	procSettingsLineTo.Call(uintptr(hdc), uintptr(left+20), uintptr(top+32))
	procSettingsLineTo.Call(uintptr(hdc), uintptr(left+35), uintptr(top+14))
	if oldCheckPen != 0 {
		procSettingsSelectObject.Call(uintptr(hdc), oldCheckPen)
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

func (state *windowsSettingsUI) drawButton(lParam uintptr) uintptr {
	if lParam == 0 {
		return 0
	}
	item := (*settingsDrawItemStruct)(unsafe.Pointer(lParam))
	role := state.buttonRoles[item.HwndItem]
	brush := state.visual.cardBrush
	textColor := settingsRGB(29, 41, 57)
	border := state.visual.borderPen

	selectedNav := false
	if role == settingsButtonNavigation {
		for page, hwnd := range state.navButtons {
			if hwnd == item.HwndItem && page == state.page {
				selectedNav = true
				break
			}
		}
		if selectedNav {
			brush = state.visual.heroBrush
			textColor = settingsRGB(24, 89, 169)
		} else {
			brush = state.visual.sidebarBrush
			border = 0
		}
	} else if role == settingsButtonPrimary {
		brush = state.visual.accentBrush
		textColor = settingsRGB(255, 255, 255)
		if item.ItemState&settingsODSSelected != 0 {
			brush = state.visual.accentPressedBrush
		}
	}

	oldBrush, _, _ := procSettingsSelectObject.Call(uintptr(item.HDC), uintptr(brush))
	var oldPen uintptr
	if border != 0 {
		oldPen, _, _ = procSettingsSelectObject.Call(uintptr(item.HDC), uintptr(border))
	} else {
		nullPen, _, _ := procSettingsGetStock.Call(8)
		oldPen, _, _ = procSettingsSelectObject.Call(uintptr(item.HDC), nullPen)
	}
	procSettingsRoundRect.Call(
		uintptr(item.HDC),
		uintptr(item.Rect.Left), uintptr(item.Rect.Top),
		uintptr(item.Rect.Right), uintptr(item.Rect.Bottom),
		12, 12,
	)
	if oldBrush != 0 {
		procSettingsSelectObject.Call(uintptr(item.HDC), oldBrush)
	}
	if oldPen != 0 {
		procSettingsSelectObject.Call(uintptr(item.HDC), oldPen)
	}

	procSettingsSetBkMode.Call(uintptr(item.HDC), settingsTransparent)
	if item.ItemState&settingsODSDisabled != 0 {
		textColor = settingsRGB(150, 159, 172)
	}
	procSettingsSetTextColor.Call(uintptr(item.HDC), textColor)
	font, _, _ := procSettingsSendMessage.Call(uintptr(item.HwndItem), settingsWMGetFont, 0, 0)
	var oldFont uintptr
	if font != 0 {
		oldFont, _, _ = procSettingsSelectObject.Call(uintptr(item.HDC), font)
	}
	label := state.text(item.HwndItem)
	labelPtr, _ := windows.UTF16PtrFromString(label)
	rect := item.Rect
	procSettingsDrawText.Call(
		uintptr(item.HDC),
		uintptr(unsafe.Pointer(labelPtr)),
		^uintptr(0),
		uintptr(unsafe.Pointer(&rect)),
		settingsDTCenter|settingsDTVCenter|settingsDTSingleLine|settingsDTEndEllipsis,
	)
	if oldFont != 0 {
		procSettingsSelectObject.Call(uintptr(item.HDC), oldFont)
	}
	if item.ItemState&settingsODSFocus != 0 {
		focus := item.Rect
		focus.Left += 4
		focus.Top += 4
		focus.Right -= 4
		focus.Bottom -= 4
		procSettingsDrawFocusRect.Call(uintptr(item.HDC), uintptr(unsafe.Pointer(&focus)))
	}
	return 1
}

func (state *windowsSettingsUI) invalidateVisual() {
	if state.hwnd != 0 {
		procSettingsInvalidateRect.Call(uintptr(state.hwnd), 0, 1)
	}
}
