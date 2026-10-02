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

	settingsGradientFillRectH  = 0
	settingsButtonCornerRadius = 10

	settingsFWNormal   = 400
	settingsFWMedium   = 500
	settingsFWSemibold = 600
	settingsFWBold     = 700

	settingsVisualMain settingsVisualRole = iota
	settingsVisualMuted
	settingsVisualSidebar
	settingsVisualSidebarMuted
	settingsVisualHero
	settingsVisualHeroMuted
	settingsVisualCard
	settingsVisualCardMuted
	settingsVisualCardAccent
	settingsVisualCardPositive
	settingsVisualCardDanger
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
	settingsButtonHeroSecondary
	settingsButtonHeroPrimary
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

type settingsTriVertex struct {
	X     int32
	Y     int32
	Red   uint16
	Green uint16
	Blue  uint16
	Alpha uint16
}

type settingsGradientRect struct {
	UpperLeft  uint32
	LowerRight uint32
}

type settingsVisualResources struct {
	mainBrush          windows.Handle
	sidebarBrush       windows.Handle
	heroBrush          windows.Handle
	cardBrush          windows.Handle
	navSelectedBrush   windows.Handle
	successBrush       windows.Handle
	accentBrush        windows.Handle
	accentPressedBrush windows.Handle
	cyanBrush          windows.Handle
	navMutedBrush      windows.Handle
	storageTrackBrush  windows.Handle
	borderPen          windows.Handle
	navBorderPen       windows.Handle
	accentPen          windows.Handle
	glowPen            windows.Handle
	checkPen           windows.Handle
	brandFont          windows.Handle
	titleFont          windows.Handle
	headlineFont       windows.Handle
	subtitleFont       windows.Handle
	cardTitleFont      windows.Handle
	cardValueFont      windows.Handle
}

var (
	settingsMsimg32                = windows.NewLazySystemDLL("msimg32.dll")
	procSettingsBeginPaint         = settingsUser32.NewProc("BeginPaint")
	procSettingsEndPaint           = settingsUser32.NewProc("EndPaint")
	procSettingsGetClientRect      = settingsUser32.NewProc("GetClientRect")
	procSettingsFillRect           = settingsUser32.NewProc("FillRect")
	procSettingsInvalidateRect     = settingsUser32.NewProc("InvalidateRect")
	procSettingsSetTextColor       = settingsGDI32.NewProc("SetTextColor")
	procSettingsSetBkColor         = settingsGDI32.NewProc("SetBkColor")
	procSettingsSetBkMode          = settingsGDI32.NewProc("SetBkMode")
	procSettingsCreateBrush        = settingsGDI32.NewProc("CreateSolidBrush")
	procSettingsCreatePen          = settingsGDI32.NewProc("CreatePen")
	procSettingsSelectObject       = settingsGDI32.NewProc("SelectObject")
	procSettingsDeleteObject       = settingsGDI32.NewProc("DeleteObject")
	procSettingsRoundRect          = settingsGDI32.NewProc("RoundRect")
	procSettingsEllipse            = settingsGDI32.NewProc("Ellipse")
	procSettingsCreateFont         = settingsGDI32.NewProc("CreateFontW")
	procSettingsMoveToEx           = settingsGDI32.NewProc("MoveToEx")
	procSettingsLineTo             = settingsGDI32.NewProc("LineTo")
	procSettingsCreateRoundRectRgn = settingsGDI32.NewProc("CreateRoundRectRgn")
	procSettingsSelectClipRgn      = settingsGDI32.NewProc("SelectClipRgn")
	procSettingsDrawText           = settingsUser32.NewProc("DrawTextW")
	procSettingsSetWindowRgn       = settingsUser32.NewProc("SetWindowRgn")
	procSettingsGradientFill       = settingsMsimg32.NewProc("GradientFill")
)

func settingsRGB(r, g, b byte) uintptr {
	return uintptr(uint32(r) | uint32(g)<<8 | uint32(b)<<16)
}

func settingsColor16(value byte) uint16 {
	return uint16(value) << 8
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
	// Figma target: #F8FAFE main, #070B14 sidebar, #04172E → #064285 hero,
	// #0561DB primary, #1FB8FA cyan accent, #DBE5F5 card border.
	state.visual.mainBrush = windows.Handle(mustCreateBrush(248, 250, 254))
	state.visual.sidebarBrush = windows.Handle(mustCreateBrush(7, 11, 20))
	state.visual.heroBrush = windows.Handle(mustCreateBrush(5, 44, 91))
	state.visual.cardBrush = windows.Handle(mustCreateBrush(255, 255, 255))
	// Figma selected nav is rgba(#0561DB, .18) over #070B14 => approximately #071A38.
	state.visual.navSelectedBrush = windows.Handle(mustCreateBrush(7, 26, 56))
	state.visual.successBrush = windows.Handle(mustCreateBrush(20, 173, 112))
	state.visual.accentBrush = windows.Handle(mustCreateBrush(5, 97, 219))
	state.visual.accentPressedBrush = windows.Handle(mustCreateBrush(4, 76, 173))
	state.visual.cyanBrush = windows.Handle(mustCreateBrush(31, 184, 250))
	state.visual.navMutedBrush = windows.Handle(mustCreateBrush(95, 133, 173))
	state.visual.storageTrackBrush = windows.Handle(mustCreateBrush(224, 237, 250))

	borderPen, _, _ := procSettingsCreatePen.Call(0, 1, settingsRGB(219, 229, 245))
	state.visual.borderPen = windows.Handle(borderPen)
	// Figma nav border is rgba(#1FB8FA, .42) on the dark sidebar.
	navBorderPen, _, _ := procSettingsCreatePen.Call(0, 1, settingsRGB(17, 84, 117))
	state.visual.navBorderPen = windows.Handle(navBorderPen)
	accentPen, _, _ := procSettingsCreatePen.Call(0, 1, settingsRGB(31, 184, 250))
	state.visual.accentPen = windows.Handle(accentPen)
	glowPen, _, _ := procSettingsCreatePen.Call(0, 2, settingsRGB(5, 97, 219))
	state.visual.glowPen = windows.Handle(glowPen)
	checkPen, _, _ := procSettingsCreatePen.Call(0, 4, settingsRGB(255, 255, 255))
	state.visual.checkPen = windows.Handle(checkPen)

	state.visual.brandFont = createSettingsFont(-20, settingsFWBold)
	state.visual.titleFont = createSettingsFont(-22, settingsFWBold)
	state.visual.headlineFont = createSettingsFont(-20, settingsFWBold)
	state.visual.subtitleFont = createSettingsFont(-11, settingsFWNormal)
	state.visual.cardTitleFont = createSettingsFont(-10, settingsFWSemibold)
	state.visual.cardValueFont = createSettingsFont(-15, settingsFWBold)

	if state.visual.mainBrush == 0 || state.visual.sidebarBrush == 0 ||
		state.visual.heroBrush == 0 || state.visual.cardBrush == 0 ||
		state.visual.navSelectedBrush == 0 || state.visual.successBrush == 0 ||
		state.visual.accentBrush == 0 || state.visual.accentPressedBrush == 0 ||
		state.visual.cyanBrush == 0 || state.visual.navMutedBrush == 0 ||
		state.visual.storageTrackBrush == 0 || state.visual.borderPen == 0 || state.visual.navBorderPen == 0 ||
		state.visual.accentPen == 0 || state.visual.glowPen == 0 || state.visual.checkPen == 0 ||
		state.visual.brandFont == 0 || state.visual.titleFont == 0 || state.visual.headlineFont == 0 ||
		state.visual.subtitleFont == 0 || state.visual.cardTitleFont == 0 ||
		state.visual.cardValueFont == 0 {
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
		state.visual.navSelectedBrush,
		state.visual.successBrush,
		state.visual.accentBrush,
		state.visual.accentPressedBrush,
		state.visual.cyanBrush,
		state.visual.navMutedBrush,
		state.visual.storageTrackBrush,
		state.visual.borderPen,
		state.visual.navBorderPen,
		state.visual.accentPen,
		state.visual.glowPen,
		state.visual.checkPen,
		state.visual.brandFont,
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

func (state *windowsSettingsUI) applyRoundedButtonRegion(hwnd windows.Handle, width, height int32) {
	if hwnd == 0 || width <= 0 || height <= 0 {
		return
	}
	region, _, _ := procSettingsCreateRoundRectRgn.Call(
		0,
		0,
		uintptr(width+1),
		uintptr(height+1),
		uintptr(settingsButtonCornerRadius*2),
		uintptr(settingsButtonCornerRadius*2),
	)
	if region == 0 {
		return
	}
	applied, _, _ := procSettingsSetWindowRgn.Call(uintptr(hwnd), region, 1)
	if applied == 0 {
		procSettingsDeleteObject.Call(region)
	}
}

func settingsButtonRoleForName(name string) settingsButtonRole {
	switch name {
	case "overview_sync_button":
		return settingsButtonHeroPrimary
	case "overview_open_button":
		return settingsButtonHeroSecondary
	case "connect_button", "save_button", "sync_button", "schedule_save_button", "agent_enable":
		return settingsButtonPrimary
	default:
		return settingsButtonSecondary
	}
}

func (state *windowsSettingsUI) setControlFont(hwnd, font windows.Handle) {
	if hwnd == 0 || font == 0 {
		return
	}
	procSettingsSendMessage.Call(uintptr(hwnd), settingsWMSetFont, uintptr(font), 1)
}

func (state *windowsSettingsUI) visualBrush(role settingsVisualRole) windows.Handle {
	switch role {
	case settingsVisualSidebar, settingsVisualSidebarMuted:
		return state.visual.sidebarBrush
	case settingsVisualHero, settingsVisualHeroMuted:
		return state.visual.heroBrush
	case settingsVisualCard, settingsVisualCardMuted, settingsVisualCardAccent,
		settingsVisualCardPositive, settingsVisualCardDanger:
		return state.visual.cardBrush
	default:
		return state.visual.mainBrush
	}
}

func (state *windowsSettingsUI) handleStaticColor(hdc, hwnd uintptr) uintptr {
	role := state.visualRoles[windows.Handle(hwnd)]
	brush := state.visualBrush(role)
	bg := settingsRGB(248, 250, 254)
	textColor := settingsRGB(19, 28, 46)

	switch role {
	case settingsVisualMuted:
		textColor = settingsRGB(97, 115, 143)
	case settingsVisualSidebar:
		bg = settingsRGB(7, 11, 20)
		textColor = settingsRGB(255, 255, 255)
	case settingsVisualSidebarMuted:
		bg = settingsRGB(7, 11, 20)
		textColor = settingsRGB(92, 122, 161)
	case settingsVisualHero:
		bg = settingsRGB(5, 44, 91)
		textColor = settingsRGB(255, 255, 255)
	case settingsVisualHeroMuted:
		bg = settingsRGB(5, 44, 91)
		textColor = settingsRGB(184, 219, 250)
	case settingsVisualCard:
		bg = settingsRGB(255, 255, 255)
		textColor = settingsRGB(19, 28, 46)
	case settingsVisualCardMuted:
		bg = settingsRGB(255, 255, 255)
		textColor = settingsRGB(97, 115, 143)
	case settingsVisualCardAccent:
		bg = settingsRGB(255, 255, 255)
		textColor = settingsRGB(5, 97, 219)
	case settingsVisualCardPositive:
		bg = settingsRGB(255, 255, 255)
		textColor = settingsRGB(20, 145, 92)
	case settingsVisualCardDanger:
		bg = settingsRGB(255, 255, 255)
		textColor = settingsRGB(201, 79, 51)
	}

	procSettingsSetTextColor.Call(hdc, textColor)
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

	sidebar := settingsRect{Left: 0, Top: 0, Right: settingsSidebarWidth, Bottom: client.Bottom}
	procSettingsFillRect.Call(hdc, uintptr(unsafe.Pointer(&sidebar)), uintptr(state.visual.sidebarBrush))
	state.paintLogoGlow(windows.Handle(hdc))

	if state.page == settingsPageOverview {
		state.paintOverviewCards(windows.Handle(hdc))
	} else {
		state.paintRoundedPanel(windows.Handle(hdc), 212, 16, client.Right-16, client.Bottom-16, state.visual.cardBrush, state.visual.borderPen, 16)
	}
	return 0
}

func (state *windowsSettingsUI) paintLogoGlow(hdc windows.Handle) {
	state.paintRoundedPanel(hdc, 18, 18, 80, 80, state.visual.sidebarBrush, state.visual.glowPen, 16)
	state.paintRoundedPanel(hdc, 20, 20, 78, 78, state.visual.sidebarBrush, state.visual.accentPen, 14)
}

func (state *windowsSettingsUI) paintOverviewCards(hdc windows.Handle) {
	state.paintHeroGradient(hdc, 224, 88, 932, 200, 16)
	state.paintStatusCircle(hdc, 244, 108, 286, 150)

	for _, rect := range [][4]int32{
		{224, 220, 383, 312},
		{401, 220, 560, 312},
		{578, 220, 737, 312},
		{755, 220, 914, 312},
		{224, 332, 676, 574},
		{696, 332, 932, 444},
		{696, 460, 932, 590},
	} {
		state.paintRoundedPanel(hdc, rect[0], rect[1], rect[2], rect[3], state.visual.cardBrush, state.visual.borderPen, 14)
	}

	track := settingsRect{Left: 714, Top: 547, Right: 914, Bottom: 554}
	procSettingsFillRect.Call(uintptr(hdc), uintptr(unsafe.Pointer(&track)), uintptr(state.visual.storageTrackBrush))
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

func (state *windowsSettingsUI) paintHeroGradient(hdc windows.Handle, left, top, right, bottom, radius int32) {
	region, _, _ := procSettingsCreateRoundRectRgn.Call(
		uintptr(left), uintptr(top), uintptr(right+1), uintptr(bottom+1), uintptr(radius*2), uintptr(radius*2),
	)
	if region != 0 {
		procSettingsSelectClipRgn.Call(uintptr(hdc), region)
	}
	vertices := [2]settingsTriVertex{
		{
			X: left, Y: top,
			Red: settingsColor16(4), Green: settingsColor16(23), Blue: settingsColor16(46),
		},
		{
			X: right, Y: bottom,
			Red: settingsColor16(6), Green: settingsColor16(66), Blue: settingsColor16(133),
		},
	}
	gradient := settingsGradientRect{UpperLeft: 0, LowerRight: 1}
	procSettingsGradientFill.Call(
		uintptr(hdc),
		uintptr(unsafe.Pointer(&vertices[0])),
		uintptr(len(vertices)),
		uintptr(unsafe.Pointer(&gradient)),
		1,
		settingsGradientFillRectH,
	)
	if region != 0 {
		procSettingsSelectClipRgn.Call(uintptr(hdc), 0)
		procSettingsDeleteObject.Call(region)
	}
	state.paintRoundedOutline(hdc, left, top, right, bottom, state.visual.accentPen, radius)
}

func (state *windowsSettingsUI) paintStatusCircle(hdc windows.Handle, left, top, right, bottom int32) {
	oldBrush, _, _ := procSettingsSelectObject.Call(uintptr(hdc), uintptr(state.visual.successBrush))
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
	procSettingsMoveToEx.Call(uintptr(hdc), uintptr(left+11), uintptr(top+22), 0)
	procSettingsLineTo.Call(uintptr(hdc), uintptr(left+18), uintptr(top+29))
	procSettingsLineTo.Call(uintptr(hdc), uintptr(left+32), uintptr(top+13))
	if oldCheckPen != 0 {
		procSettingsSelectObject.Call(uintptr(hdc), oldCheckPen)
	}
}

func (state *windowsSettingsUI) paintRoundedPanel(hdc windows.Handle, left, top, right, bottom int32, brush, pen windows.Handle, radius int32) {
	oldBrush, _, _ := procSettingsSelectObject.Call(uintptr(hdc), uintptr(brush))
	oldPen, _, _ := procSettingsSelectObject.Call(uintptr(hdc), uintptr(pen))
	procSettingsRoundRect.Call(
		uintptr(hdc),
		uintptr(left), uintptr(top), uintptr(right), uintptr(bottom),
		uintptr(radius*2), uintptr(radius*2),
	)
	if oldBrush != 0 {
		procSettingsSelectObject.Call(uintptr(hdc), oldBrush)
	}
	if oldPen != 0 {
		procSettingsSelectObject.Call(uintptr(hdc), oldPen)
	}
}

func (state *windowsSettingsUI) paintRoundedOutline(hdc windows.Handle, left, top, right, bottom int32, pen windows.Handle, radius int32) {
	hollowBrush, _, _ := procSettingsGetStock.Call(5)
	oldBrush, _, _ := procSettingsSelectObject.Call(uintptr(hdc), hollowBrush)
	oldPen, _, _ := procSettingsSelectObject.Call(uintptr(hdc), uintptr(pen))
	procSettingsRoundRect.Call(
		uintptr(hdc),
		uintptr(left), uintptr(top), uintptr(right), uintptr(bottom),
		uintptr(radius*2), uintptr(radius*2),
	)
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
	backgroundBrush := state.visual.cardBrush
	textColor := settingsRGB(19, 28, 46)
	border := state.visual.borderPen
	selected := false

	switch role {
	case settingsButtonNavigation:
		backgroundBrush = state.visual.sidebarBrush
		for page, hwnd := range state.navButtons {
			if hwnd == item.HwndItem && page == state.page {
				selected = true
				break
			}
		}
		if selected {
			brush = state.visual.navSelectedBrush
			textColor = settingsRGB(255, 255, 255)
			border = state.visual.navBorderPen
		} else {
			brush = state.visual.sidebarBrush
			textColor = settingsRGB(184, 204, 227)
			border = 0
		}
	case settingsButtonHeroPrimary:
		backgroundBrush = state.visual.heroBrush
		brush = state.visual.accentBrush
		textColor = settingsRGB(255, 255, 255)
		border = 0
		if item.ItemState&settingsODSSelected != 0 {
			brush = state.visual.accentPressedBrush
		}
	case settingsButtonHeroSecondary:
		backgroundBrush = state.visual.heroBrush
		brush = state.visual.cardBrush
		textColor = settingsRGB(19, 28, 46)
		border = state.visual.borderPen
	case settingsButtonPrimary:
		brush = state.visual.accentBrush
		textColor = settingsRGB(255, 255, 255)
		border = 0
		if item.ItemState&settingsODSSelected != 0 {
			brush = state.visual.accentPressedBrush
		}
	}

	// BUTTON controls are rectangular Win32 child windows. Clear the complete
	// client rectangle with the parent surface before painting the rounded
	// shape so the native button-face background cannot leak through corners.
	procSettingsFillRect.Call(
		uintptr(item.HDC),
		uintptr(unsafe.Pointer(&item.Rect)),
		uintptr(backgroundBrush),
	)

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
		settingsButtonCornerRadius*2, settingsButtonCornerRadius*2,
	)

	if role == settingsButtonNavigation {
		dotBrush := state.visual.navMutedBrush
		if selected {
			dotBrush = state.visual.cyanBrush
		}
		oldDotBrush, _, _ := procSettingsSelectObject.Call(uintptr(item.HDC), uintptr(dotBrush))
		nullPen, _, _ := procSettingsGetStock.Call(8)
		oldDotPen, _, _ := procSettingsSelectObject.Call(uintptr(item.HDC), nullPen)
		procSettingsEllipse.Call(
			uintptr(item.HDC),
			uintptr(item.Rect.Left+13), uintptr(item.Rect.Top+14),
			uintptr(item.Rect.Left+19), uintptr(item.Rect.Top+20),
		)
		if oldDotBrush != 0 {
			procSettingsSelectObject.Call(uintptr(item.HDC), oldDotBrush)
		}
		if oldDotPen != 0 {
			procSettingsSelectObject.Call(uintptr(item.HDC), oldDotPen)
		}
	}

	if oldBrush != 0 {
		procSettingsSelectObject.Call(uintptr(item.HDC), oldBrush)
	}
	if oldPen != 0 {
		procSettingsSelectObject.Call(uintptr(item.HDC), oldPen)
	}

	procSettingsSetBkMode.Call(uintptr(item.HDC), settingsTransparent)
	if item.ItemState&settingsODSDisabled != 0 {
		textColor = settingsRGB(133, 150, 173)
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
	flags := uintptr(settingsDTCenter | settingsDTVCenter | settingsDTSingleLine | settingsDTEndEllipsis)
	if role == settingsButtonNavigation {
		// Figma uses a 30 px left text inset, not centered navigation labels.
		rect.Left += 30
		rect.Right -= 8
		flags = settingsDTVCenter | settingsDTSingleLine | settingsDTEndEllipsis
	}
	procSettingsDrawText.Call(
		uintptr(item.HDC),
		uintptr(unsafe.Pointer(labelPtr)),
		^uintptr(0),
		uintptr(unsafe.Pointer(&rect)),
		flags,
	)
	if oldFont != 0 {
		procSettingsSelectObject.Call(uintptr(item.HDC), oldFont)
	}

	if item.ItemState&settingsODSFocus != 0 {
		// Draw a deterministic rounded focus ring instead of DrawFocusRect,
		// whose XOR rendering produced the double/uneven outline seen live.
		focus := item.Rect
		focus.Left += 3
		focus.Top += 3
		focus.Right -= 3
		focus.Bottom -= 3
		state.paintRoundedOutline(
			item.HDC,
			focus.Left,
			focus.Top,
			focus.Right,
			focus.Bottom,
			state.visual.accentPen,
			settingsButtonCornerRadius-3,
		)
	}
	return 1
}

func (state *windowsSettingsUI) invalidateVisual() {
	if state.hwnd != 0 {
		procSettingsInvalidateRect.Call(uintptr(state.hwnd), 0, 1)
	}
}
