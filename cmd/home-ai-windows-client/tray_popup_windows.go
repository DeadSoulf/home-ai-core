//go:build windows

package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/DeadSoulf/home-ai-core/internal/windowsclient"
	"golang.org/x/sys/windows"
)

const (
	trayPopupClassName = "HomeAIWindowsTrayPopup"

	trayPopupWSChild   = 0x40000000
	trayPopupWSVisible = 0x10000000
	trayPopupWSPopup   = 0x80000000
	trayPopupWSBorder  = 0x00800000
	trayPopupWSTabStop = 0x00010000
	trayPopupBSFlat    = 0x00008000
	trayPopupSSIcon    = 0x00000003

	trayPopupEXTopMost = 0x00000008
	trayPopupEXToolWin = 0x00000080

	trayPopupWMActivate = 0x0006
	trayPopupWAInactive = 0
	trayPopupSWShow     = 5
	trayPopupSTMSetIcon = 0x0170
)

var (
	trayPopupClassOnce sync.Once
	trayPopupClassErr  error

	procTrayPopupShowWindow   = trayUser32.NewProc("ShowWindow")
	procTrayPopupUpdateWindow = trayUser32.NewProc("UpdateWindow")
)

func (state *windowsAgentTray) showStatusPopup() {
	if state.popup != 0 {
		procDestroyWindow.Call(uintptr(state.popup))
		return
	}
	if err := ensureTrayPopupClass(); err != nil {
		state.showMenu()
		return
	}

	var point trayPoint
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&point)))
	x := point.X - 390
	y := point.Y - 430
	if x < 8 {
		x = 8
	}
	if y < 8 {
		y = 8
	}

	className, _ := windows.UTF16PtrFromString(trayPopupClassName)
	windowName, _ := windows.UTF16PtrFromString("HOME AI")
	module, _, _ := procGetModuleHandleW.Call(0)
	hwnd, _, _ := procCreateWindowExW.Call(
		trayPopupEXTopMost|trayPopupEXToolWin,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowName)),
		trayPopupWSPopup|trayPopupWSBorder,
		uintptr(x),
		uintptr(y),
		390,
		410,
		0,
		0,
		module,
		0,
	)
	if hwnd == 0 {
		state.showMenu()
		return
	}
	state.popup = windows.Handle(hwnd)
	state.createStatusPopupControls(windows.Handle(module))
	state.refreshStatusPopup()
	procSetForegroundWindow.Call(hwnd)
	procTrayPopupShowWindow.Call(hwnd, trayPopupSWShow)
	procTrayPopupUpdateWindow.Call(hwnd)
}

func ensureTrayPopupClass() error {
	trayPopupClassOnce.Do(func() {
		className, _ := windows.UTF16PtrFromString(trayPopupClassName)
		module, _, moduleErr := procGetModuleHandleW.Call(0)
		if module == 0 {
			trayPopupClassErr = fmt.Errorf("get tray popup module handle: %w", moduleErr)
			return
		}
		cursor, _, _ := procLoadCursorW.Call(0, idcArrow)
		class := trayWndClassEx{
			CbSize:        uint32(unsafe.Sizeof(trayWndClassEx{})),
			LpfnWndProc:   syscall.NewCallback(trayPopupWindowProc),
			HInstance:     windows.Handle(module),
			HCursor:       windows.Handle(cursor),
			HbrBackground: windows.Handle(6),
			LpszClassName: className,
		}
		atom, _, registerErr := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&class)))
		if atom == 0 {
			trayPopupClassErr = fmt.Errorf("register tray popup class: %w", registerErr)
		}
	})
	return trayPopupClassErr
}

func (state *windowsAgentTray) createStatusPopupControls(module windows.Handle) {
	font, _, _ := procSettingsGetStock.Call(settingsDefaultGUIFont)
	create := func(class, text string, style uint32, x, y, w, h int32, id uint16) windows.Handle {
		classPtr, _ := windows.UTF16PtrFromString(class)
		textPtr, _ := windows.UTF16PtrFromString(text)
		hwnd, _, _ := procCreateWindowExW.Call(
			0,
			uintptr(unsafe.Pointer(classPtr)),
			uintptr(unsafe.Pointer(textPtr)),
			uintptr(style),
			uintptr(x), uintptr(y), uintptr(w), uintptr(h),
			uintptr(state.popup),
			uintptr(id),
			uintptr(module),
			0,
		)
		if hwnd != 0 && font != 0 {
			procSettingsSendMessage.Call(hwnd, settingsWMSetFont, font, 1)
		}
		return windows.Handle(hwnd)
	}

	if state.icon != 0 {
		icon := create("STATIC", "", trayPopupWSChild|trayPopupWSVisible|trayPopupSSIcon, 20, 18, 34, 34, 0)
		if icon != 0 {
			procSettingsSendMessage.Call(uintptr(icon), trayPopupSTMSetIcon, uintptr(state.icon), 0)
		}
	}
	create("STATIC", "HOME AI", trayPopupWSChild|trayPopupWSVisible, 66, 22, 150, 26, 0)
	state.popupStatus = create("STATIC", "", trayPopupWSChild|trayPopupWSVisible, 20, 70, 340, 32, 0)
	state.popupSubtitle = create("STATIC", "", trayPopupWSChild|trayPopupWSVisible, 20, 104, 340, 38, 0)

	create("STATIC", textForLanguage(preferredUILanguage(), "recent_activity"), trayPopupWSChild|trayPopupWSVisible, 20, 158, 220, 24, 0)
	for i := range state.popupRows {
		state.popupRows[i] = create("STATIC", "", trayPopupWSChild|trayPopupWSVisible, 22, int32(188+i*46), 336, 40, 0)
	}

	create("BUTTON", textForLanguage(preferredUILanguage(), "sync_now"), trayPopupWSChild|trayPopupWSVisible|trayPopupWSTabStop|trayPopupBSFlat, 20, 330, 168, 34, traySyncNow)
	create("BUTTON", textForLanguage(preferredUILanguage(), "open_home_folder"), trayPopupWSChild|trayPopupWSVisible|trayPopupWSTabStop|trayPopupBSFlat, 202, 330, 168, 34, trayOpenFolder)
	create("BUTTON", textForLanguage(preferredUILanguage(), "settings"), trayPopupWSChild|trayPopupWSVisible|trayPopupWSTabStop|trayPopupBSFlat, 202, 370, 168, 28, traySettings)
}

func trayPopupWindowProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	activeWindowsTray.Lock()
	state := activeWindowsTray.value
	activeWindowsTray.Unlock()

	switch message {
	case trayPopupWMActivate:
		if uint16(wParam&0xffff) == trayPopupWAInactive {
			procDestroyWindow.Call(hwnd)
			return 0
		}
	case wmCommand:
		if state != nil {
			state.handleTrayCommand(uint16(wParam & 0xffff))
			if state.popup != 0 {
				procDestroyWindow.Call(uintptr(state.popup))
			}
		}
		return 0
	case wmClose:
		procDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		if state != nil && uintptr(state.popup) == hwnd {
			state.popup = 0
			state.popupStatus = 0
			state.popupSubtitle = 0
			state.popupRows = [3]windows.Handle{}
		}
		return 0
	}
	result, _, _ := procDefWindowProcW.Call(hwnd, uintptr(message), wParam, lParam)
	return result
}

func (state *windowsAgentTray) handleTrayCommand(id uint16) {
	switch id {
	case traySyncNow:
		select {
		case state.runNow <- struct{}{}:
		default:
		}
	case trayOpenLog:
		state.openPath(state.logPath)
	case trayOpenConfig:
		state.openPath(state.configPath)
	case trayOpenFolder:
		if path := primaryLocalFolderForConfig(state.configPath); path != "" {
			state.openPath(path)
		}
	case traySettings:
		_ = startSettingsProcess(state.configPath)
	case trayExit:
		state.signalExit()
		if state.hwnd != 0 {
			procDestroyWindow.Call(uintptr(state.hwnd))
		}
	}
}

func (state *windowsAgentTray) refreshStatusPopup() {
	if state.popup == 0 || state.popupStatus == 0 {
		return
	}
	language := preferredUILanguage()
	headline, subtitle, rows := trayPopupSnapshot(state.configPath, language)
	setTrayPopupText(state.popupStatus, headline)
	setTrayPopupText(state.popupSubtitle, subtitle)
	for i := range state.popupRows {
		setTrayPopupText(state.popupRows[i], rows[i])
	}
}

func setTrayPopupText(hwnd windows.Handle, text string) {
	if hwnd == 0 {
		return
	}
	value, _ := windows.UTF16PtrFromString(text)
	procSettingsSetWindowText.Call(uintptr(hwnd), uintptr(unsafe.Pointer(value)))
}

func trayPopupSnapshot(configPath, language string) (string, string, [3]string) {
	profiles, err := windowsclient.LoadSyncProfiles(configPath)
	if err != nil {
		return textForLanguage(language, "overview_attention"), err.Error(), [3]string{}
	}
	enabled := 0
	failed := 0
	var lastSuccess time.Time
	for _, profile := range profiles {
		if !profile.Enabled {
			continue
		}
		enabled++
		if profile.LastError != "" {
			failed++
		}
		if profile.LastSuccessAt != nil && profile.LastSuccessAt.After(lastSuccess) {
			lastSuccess = profile.LastSuccessAt.Local()
		}
	}

	headline := textForLanguage(language, "overview_not_configured")
	subtitle := textForLanguage(language, "overview_hint")
	switch {
	case len(profiles) == 0:
	case failed > 0:
		headline = textForLanguage(language, "overview_attention")
		subtitle = textForLanguagef(language, "overview_failed_count", failed)
	case enabled == 0:
		headline = textForLanguage(language, "overview_paused")
		subtitle = textForLanguage(language, "overview_no_enabled")
	case lastSuccess.IsZero():
		headline = textForLanguage(language, "overview_ready")
		subtitle = textForLanguagef(language, "overview_enabled_count", enabled)
	default:
		headline = textForLanguage(language, "overview_all_synced")
		subtitle = textForLanguage(language, "overview_files_current")
	}

	sort.SliceStable(profiles, func(i, j int) bool {
		var left, right time.Time
		if profiles[i].LastAttemptAt != nil {
			left = profiles[i].LastAttemptAt.Local()
		}
		if profiles[j].LastAttemptAt != nil {
			right = profiles[j].LastAttemptAt.Local()
		}
		return left.After(right)
	})
	var rows [3]string
	index := 0
	for _, profile := range profiles {
		if index >= len(rows) {
			break
		}
		if profile.LastAttemptAt == nil {
			continue
		}
		name := filepath.Base(profile.Source)
		if strings.TrimSpace(name) == "" || name == "." {
			name = profile.Source
		}
		status := textForLanguage(language, "recent_activity_ok")
		if profile.LastError != "" {
			status = textForLanguage(language, "recent_activity_error")
		}
		rows[index] = textForLanguagef(language, "recent_activity_row", name, status, dashboardTime(profile.LastAttemptAt.Local(), language))
		index++
	}
	for index < len(rows) {
		rows[index] = textForLanguage(language, "recent_activity_empty")
		index++
	}
	return headline, subtitle, rows
}
