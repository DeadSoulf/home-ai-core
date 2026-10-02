//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/windowsclient"
	"golang.org/x/sys/windows"
)

const (
	settingsPageOverview = iota
	settingsPageConnection
	settingsPageSync
	settingsPageBackup
	settingsPageGeneral
	settingsPageCount
)

const (
	settingsCompactWindowWidth  = 976
	settingsCompactWindowHeight = 635
	settingsSidebarWidth        = 196
)

const (
	settingsIDNavOverview   = 2101
	settingsIDNavConnection = 2102
	settingsIDNavSync       = 2103
	settingsIDNavBackup     = 2104
	settingsIDNavGeneral    = 2105
	settingsIDOpenLocal     = 2106
	settingsIDBackupProfile = 2107
	settingsIDSaveSchedule  = 2108

	settingsWSGroup      = 0x00020000
	settingsBSAutoRadio  = 0x00000009
	settingsBSPushLike   = 0x00001000
	settingsBSFlat       = 0x00008000
	settingsBMSetCheck   = 0x00F1
	settingsBSTUnchecked = 0
	settingsBSTChecked   = 1
	settingsSSIcon       = 0x00000003
	settingsSTMSetIcon   = 0x0170
	settingsWMKeyDown    = 0x0100
	settingsVKTab        = 0x09
	settingsVKShift      = 0x10
	settingsVKControl    = 0x11
	settingsVKLeft       = 0x25
	settingsVKUp         = 0x26
	settingsVKRight      = 0x27
	settingsVKDown       = 0x28
)

var (
	procSettingsIsDialogMessage = settingsUser32.NewProc("IsDialogMessageW")
	procSettingsGetKeyState     = settingsUser32.NewProc("GetKeyState")
	procSettingsGetFocus        = settingsUser32.NewProc("GetFocus")
	procSettingsSetFocus        = settingsUser32.NewProc("SetFocus")
)

func (state *windowsSettingsUI) createLightControls(module windows.Handle) error {
	font, _, _ := procSettingsGetStock.Call(settingsDefaultGUIFont)

	staticText := func(name, text string, x, y, w, h int32, page int, role settingsVisualRole) windows.Handle {
		hwnd := state.createControl(module, "STATIC", text, settingsWSChild|settingsWSVisible, x, y, w, h, 0, windows.Handle(font))
		if name != "" {
			state.localized[name] = hwnd
		}
		state.setVisualRole(hwnd, role)
		if page >= 0 {
			state.trackPage(page, hwnd)
		}
		return hwnd
	}
	static := func(name, key string, x, y, w, h int32, page int, role settingsVisualRole) windows.Handle {
		return staticText(name, state.tr(key), x, y, w, h, page, role)
	}
	edit := func(id uint16, text string, x, y, w, h int32, extra uint32, page int) windows.Handle {
		hwnd := state.createControl(module, "EDIT", text, settingsWSChild|settingsWSVisible|settingsWSTabStop|settingsWSBorder|settingsESAutoHScroll|extra, x, y, w, h, id, windows.Handle(font))
		state.trackPage(page, hwnd)
		return hwnd
	}
	button := func(name string, id uint16, key string, x, y, w, h int32, page int) windows.Handle {
		hwnd := state.createControl(module, "BUTTON", state.tr(key), settingsWSChild|settingsWSVisible|settingsWSTabStop|settingsBSOwnerDraw, x, y, w, h, id, windows.Handle(font))
		if name != "" {
			state.localized[name] = hwnd
		}
		state.buttonRoles[hwnd] = settingsButtonRoleForName(name)
		state.applyRoundedButtonRegion(hwnd, w, h)
		if page >= 0 {
			state.trackPage(page, hwnd)
		}
		return hwnd
	}
	combo := func(id uint16, x, y, w, h int32, page int) windows.Handle {
		hwnd := state.createControl(module, "COMBOBOX", "", settingsWSChild|settingsWSVisible|settingsWSTabStop|settingsWSVScroll|settingsCBSDropDownList, x, y, w, h, id, windows.Handle(font))
		state.trackPage(page, hwnd)
		return hwnd
	}

	// Compact HOME AI sidebar from the approved Figma layout.
	if state.windowIcon != 0 {
		iconView := state.createControl(module, "STATIC", "", settingsWSChild|settingsWSVisible|settingsSSIcon, 25, 25, 48, 48, 0, windows.Handle(font))
		if iconView != 0 {
			state.setVisualRole(iconView, settingsVisualSidebar)
			procSettingsSendMessage.Call(uintptr(iconView), settingsSTMSetIcon, uintptr(state.windowIcon), 0)
		}
	}
	brand := static("brand_name", "brand_name", 88, 31, 104, 25, -1, settingsVisualSidebar)
	state.setControlFont(brand, state.visual.brandFont)
	staticText("brand_desktop", "Desktop", 90, 58, 96, 18, -1, settingsVisualSidebarMuted)
	staticText("", "© 2026 TexNik", 18, 560, 150, 20, -1, settingsVisualSidebarMuted)

	nav := func(page int, id uint16, key string, y int32, first bool) {
		style := uint32(settingsWSChild | settingsWSVisible | settingsWSTabStop | settingsBSOwnerDraw)
		if first {
			style |= settingsWSGroup
		}
		hwnd := state.createControl(module, "BUTTON", state.tr(key), style, 16, y, 164, 36, id, windows.Handle(font))
		state.applyRoundedButtonRegion(hwnd, 164, 36)
		state.navButtons[page] = hwnd
		state.buttonRoles[hwnd] = settingsButtonNavigation
		state.localized[fmt.Sprintf("nav_%d", page)] = hwnd
	}
	nav(settingsPageOverview, settingsIDNavOverview, "nav_overview", 112, true)
	nav(settingsPageConnection, settingsIDNavConnection, "nav_connection", 156, false)
	nav(settingsPageSync, settingsIDNavSync, "nav_sync", 200, false)
	nav(settingsPageBackup, settingsIDNavBackup, "nav_backup", 244, false)
	nav(settingsPageGeneral, settingsIDNavGeneral, "nav_settings", 288, false)

	// Overview — exact compact geometry from Figma node 2:2.
	title := static("overview_title", "nav_overview", 224, 24, 360, 28, settingsPageOverview, settingsVisualMain)
	state.setControlFont(title, state.visual.titleFont)
	static("overview_main_subtitle", "overview_main_subtitle", 225, 55, 420, 20, settingsPageOverview, settingsVisualMuted)

	state.overviewHeadline = static("", "overview_not_configured", 301, 107, 395, 26, settingsPageOverview, settingsVisualHero)
	state.setControlFont(state.overviewHeadline, state.visual.headlineFont)
	state.overviewSubtitle = static("", "overview_hint", 302, 139, 390, 20, settingsPageOverview, settingsVisualHeroMuted)
	state.setControlFont(state.overviewSubtitle, state.visual.subtitleFont)
	button("overview_sync_button", settingsIDSyncNow, "sync_compact", 733, 105, 181, 34, settingsPageOverview)
	button("overview_open_button", settingsIDOpenLocal, "open_home_folder", 733, 149, 181, 34, settingsPageOverview)

	// Four compact status cards.
	connTitle := static("overview_connection_title", "overview_connection", 242, 234, 128, 16, settingsPageOverview, settingsVisualCardMuted)
	state.setControlFont(connTitle, state.visual.cardTitleFont)
	state.overviewConnection = staticText("", state.tr("overview_not_configured"), 242, 258, 128, 20, settingsPageOverview, settingsVisualCard)
	state.setControlFont(state.overviewConnection, state.visual.cardValueFont)
	state.overviewConnectionHint = staticText("", "", 242, 284, 128, 16, settingsPageOverview, settingsVisualCardAccent)

	folderTitle := static("overview_folder_title", "overview_folder", 419, 234, 128, 16, settingsPageOverview, settingsVisualCardMuted)
	state.setControlFont(folderTitle, state.visual.cardTitleFont)
	state.overviewSync = staticText("", state.tr("overview_folder_none"), 419, 258, 128, 20, settingsPageOverview, settingsVisualCard)
	state.setControlFont(state.overviewSync, state.visual.cardValueFont)
	state.overviewSyncHint = staticText("", "", 419, 284, 128, 16, settingsPageOverview, settingsVisualCardMuted)

	lastTitle := static("overview_last_title", "overview_last_short", 596, 234, 128, 16, settingsPageOverview, settingsVisualCardMuted)
	state.setControlFont(lastTitle, state.visual.cardTitleFont)
	state.overviewLast = staticText("", state.tr("overview_not_synced"), 596, 258, 128, 20, settingsPageOverview, settingsVisualCard)
	state.setControlFont(state.overviewLast, state.visual.cardValueFont)

	nextTitle := static("overview_next_title", "overview_next", 773, 234, 128, 16, settingsPageOverview, settingsVisualCardMuted)
	state.setControlFont(nextTitle, state.visual.cardTitleFont)
	state.overviewNext = staticText("", state.tr("overview_next_manual_short"), 773, 258, 128, 20, settingsPageOverview, settingsVisualCard)
	state.setControlFont(state.overviewNext, state.visual.cardValueFont)

	recentTitle := static("overview_recent_title", "recent_activity", 242, 348, 300, 24, settingsPageOverview, settingsVisualCard)
	state.setControlFont(recentTitle, state.visual.cardValueFont)
	for i := range state.overviewRecent {
		state.overviewRecent[i] = staticText("", state.tr("recent_activity_empty"), 242, int32(384+i*46), 416, 38, settingsPageOverview, settingsVisualCardMuted)
	}
	static("overview_recent_hint", "recent_activity_hint", 242, 536, 360, 16, settingsPageOverview, settingsVisualCardMuted)

	backupTitle := static("overview_backup_title", "backup_summary", 714, 348, 190, 18, settingsPageOverview, settingsVisualCardMuted)
	state.setControlFont(backupTitle, state.visual.cardTitleFont)
	static("overview_backup_label", "backup_background", 714, 376, 190, 18, settingsPageOverview, settingsVisualCard)
	state.overviewBackup = staticText("", state.tr("backup_off"), 714, 404, 190, 18, settingsPageOverview, settingsVisualCardDanger)
	state.setControlFont(state.overviewBackup, state.visual.cardTitleFont)

	storageTitle := static("overview_storage_title", "storage_summary", 714, 476, 190, 18, settingsPageOverview, settingsVisualCardMuted)
	state.setControlFont(storageTitle, state.visual.cardTitleFont)
	state.overviewStorage = staticText("", state.tr("storage_unknown"), 714, 504, 190, 20, settingsPageOverview, settingsVisualCard)
	state.setControlFont(state.overviewStorage, state.visual.cardTitleFont)
	state.overviewStorageHint = staticText("", "", 714, 562, 200, 16, settingsPageOverview, settingsVisualCardMuted)

	// Connection.
	connectionTitle := static("connection_title", "nav_connection", 224, 32, 320, 28, settingsPageConnection, settingsVisualCard)
	state.setControlFont(connectionTitle, state.visual.titleFont)
	static("connection_hint", "connection_hint", 224, 72, 680, 38, settingsPageConnection, settingsVisualCardMuted)
	static("server_label", "server", 224, 126, 120, 20, settingsPageConnection, settingsVisualCardMuted)
	state.serverEdit = edit(settingsIDServerEdit, "", 224, 150, 520, 30, 0, settingsPageConnection)
	static("user_label", "user", 224, 204, 120, 20, settingsPageConnection, settingsVisualCardMuted)
	state.usernameEdit = edit(settingsIDUsernameEdit, "", 224, 228, 250, 30, 0, settingsPageConnection)
	static("password_label", "password", 500, 204, 120, 20, settingsPageConnection, settingsVisualCardMuted)
	state.passwordEdit = edit(settingsIDPasswordEdit, "", 500, 228, 244, 30, settingsESPassword, settingsPageConnection)
	static("password_hint", "password_hint", 224, 270, 520, 20, settingsPageConnection, settingsVisualCardMuted)
	button("connect_button", settingsIDConnect, "connect_save", 224, 312, 220, 36, settingsPageConnection)

	// Synchronization.
	syncTitle := static("sync_title", "nav_sync", 224, 32, 340, 28, settingsPageSync, settingsVisualCard)
	state.setControlFont(syncTitle, state.visual.titleFont)
	static("sync_hint", "sync_hint", 224, 72, 680, 36, settingsPageSync, settingsVisualCardMuted)
	static("remote_label", "remote_folder", 224, 116, 180, 20, settingsPageSync, settingsVisualCardMuted)
	state.folderCombo = combo(settingsIDFolderCombo, 224, 140, 680, 200, settingsPageSync)
	static("local_label", "local_source", 224, 190, 180, 20, settingsPageSync, settingsVisualCardMuted)
	state.sourceEdit = edit(settingsIDSourceEdit, "", 224, 214, 568, 30, 0, settingsPageSync)
	button("browse_button", settingsIDBrowse, "browse", 804, 213, 100, 32, settingsPageSync)
	static("destination_label", "destination", 224, 258, 180, 20, settingsPageSync, settingsVisualCardMuted)
	state.destination = edit(settingsIDDestination, "", 224, 282, 680, 30, 0, settingsPageSync)

	state.saveProfileBtn = button("save_button", settingsIDSaveProfile, "add_profile", 224, 326, 140, 34, settingsPageSync)
	button("new_button", settingsIDNewProfile, "new_clear", 374, 326, 126, 34, settingsPageSync)
	button("enable_button", settingsIDEnableProfile, "enable", 510, 326, 96, 34, settingsPageSync)
	button("disable_button", settingsIDDisableProfile, "disable", 616, 326, 96, 34, settingsPageSync)
	button("delete_button", settingsIDDeleteProfile, "delete", 722, 326, 86, 34, settingsPageSync)
	button("refresh_button", settingsIDRefresh, "refresh", 818, 326, 86, 34, settingsPageSync)

	static("profiles_label", "sync_profiles", 224, 374, 300, 20, settingsPageSync, settingsVisualCardMuted)
	state.profileList = state.createControl(module, "LISTBOX", "", settingsWSChild|settingsWSVisible|settingsWSTabStop|settingsWSBorder|settingsWSVScroll|settingsLBSNotify, 224, 398, 680, 120, settingsIDProfileList, windows.Handle(font))
	state.trackPage(settingsPageSync, state.profileList)
	button("sync_button", settingsIDSyncNow, "sync_now", 704, 532, 200, 34, settingsPageSync)

	// Backups / schedule.
	backupPageTitle := static("backup_title", "nav_backup", 224, 32, 340, 28, settingsPageBackup, settingsVisualCard)
	state.setControlFont(backupPageTitle, state.visual.titleFont)
	static("backup_hint", "backup_hint", 224, 72, 680, 40, settingsPageBackup, settingsVisualCardMuted)
	static("backup_profile_label", "backup_folder", 224, 124, 190, 20, settingsPageBackup, settingsVisualCardMuted)
	state.backupProfileCombo = combo(settingsIDBackupProfile, 224, 148, 680, 180, settingsPageBackup)
	static("interval_label", "interval", 224, 198, 180, 20, settingsPageBackup, settingsVisualCardMuted)
	state.interval = edit(settingsIDInterval, "15m", 224, 222, 220, 30, 0, settingsPageBackup)
	static("conflict_label", "conflict", 476, 198, 180, 20, settingsPageBackup, settingsVisualCardMuted)
	state.conflict = combo(settingsIDConflict, 476, 222, 300, 150, settingsPageBackup)
	state.comboAdd(state.conflict, state.tr("conflict_stop"))
	state.comboAdd(state.conflict, state.tr("conflict_skip"))
	state.comboAdd(state.conflict, state.tr("conflict_replace"))
	procSettingsSendMessage.Call(uintptr(state.conflict), settingsCBSetCurSel, 0, 0)
	button("schedule_save_button", settingsIDSaveSchedule, "save_schedule", 224, 270, 220, 34, settingsPageBackup)

	static("backup_agent_title", "backup_agent_title", 224, 326, 260, 20, settingsPageBackup, settingsVisualCard)
	state.agentLabel = static("", "autostart_checking", 224, 354, 680, 44, settingsPageBackup, settingsVisualCardMuted)
	button("agent_enable", settingsIDAgentEnable, "enable_agent", 224, 418, 180, 34, settingsPageBackup)
	button("agent_disable", settingsIDAgentDisable, "disable_agent", 416, 418, 180, 34, settingsPageBackup)

	// General settings.
	generalTitle := static("general_title", "nav_settings", 224, 32, 340, 28, settingsPageGeneral, settingsVisualCard)
	state.setControlFont(generalTitle, state.visual.titleFont)
	static("general_hint", "general_hint", 224, 72, 680, 36, settingsPageGeneral, settingsVisualCardMuted)
	static("language_label", "language", 224, 126, 180, 20, settingsPageGeneral, settingsVisualCardMuted)
	state.languageCombo = combo(settingsIDLanguage, 224, 150, 300, 120, settingsPageGeneral)
	state.comboAdd(state.languageCombo, "Русский")
	state.comboAdd(state.languageCombo, "English")
	if state.language == uiLanguageRussian {
		procSettingsSendMessage.Call(uintptr(state.languageCombo), settingsCBSetCurSel, 0, 0)
	} else {
		procSettingsSendMessage.Call(uintptr(state.languageCombo), settingsCBSetCurSel, 1, 0)
	}
	static("general_background_title", "general_background_title", 224, 224, 260, 22, settingsPageGeneral, settingsVisualCard)
	static("general_background_hint", "general_background_hint", 224, 254, 680, 64, settingsPageGeneral, settingsVisualCardMuted)

	// Global status is hidden on Overview and visible on functional pages.
	static("status_title", "status", 224, 560, 74, 20, -1, settingsVisualMuted)
	state.statusLabel = state.createControl(module, "STATIC", state.tr("ready"), settingsWSChild|settingsWSVisible, 302, 560, 602, 20, 0, windows.Handle(font))
	state.setVisualRole(state.statusLabel, settingsVisualMuted)

	if state.serverEdit == 0 || state.usernameEdit == 0 || state.passwordEdit == 0 ||
		state.folderCombo == 0 || state.sourceEdit == 0 || state.destination == 0 ||
		state.interval == 0 || state.conflict == 0 || state.profileList == 0 ||
		state.backupProfileCombo == 0 || state.statusLabel == 0 || state.agentLabel == 0 ||
		state.languageCombo == 0 || state.overviewStorage == 0 {
		return fmt.Errorf("create Windows client controls")
	}

	state.showPage(settingsPageOverview)
	return nil
}

func (state *windowsSettingsUI) trackPage(page int, handles ...windows.Handle) {
	if page < 0 || page >= settingsPageCount {
		return
	}
	for _, hwnd := range handles {
		if hwnd != 0 {
			state.pageControls[page] = append(state.pageControls[page], hwnd)
		}
	}
}

func (state *windowsSettingsUI) showPage(page int) {
	if page < 0 || page >= settingsPageCount {
		page = settingsPageOverview
	}
	state.page = page
	for candidate := 0; candidate < settingsPageCount; candidate++ {
		command := uintptr(0)
		if candidate == page {
			command = settingsSWShow
		}
		for _, hwnd := range state.pageControls[candidate] {
			procSettingsShowWindow.Call(uintptr(hwnd), command)
		}
		if nav := state.navButtons[candidate]; nav != 0 {
			checked := uintptr(settingsBSTUnchecked)
			if candidate == page {
				checked = settingsBSTChecked
			}
			procSettingsSendMessage.Call(uintptr(nav), settingsBMSetCheck, checked, 0)
		}
	}
	if page == settingsPageBackup && state.selectedProfileID == "" && len(state.profiles) > 0 {
		state.applyProfile(state.profiles[0])
		procSettingsSendMessage.Call(uintptr(state.profileList), settingsLBSetCurSel, 0, 0)
		procSettingsSendMessage.Call(uintptr(state.backupProfileCombo), settingsCBSetCurSel, 0, 0)
	}
	statusCommand := uintptr(settingsSWShow)
	if page == settingsPageOverview {
		statusCommand = 0
		state.refreshOverview()
	}
	if hwnd := state.localized["status_title"]; hwnd != 0 {
		procSettingsShowWindow.Call(uintptr(hwnd), statusCommand)
	}
	if state.statusLabel != 0 {
		procSettingsShowWindow.Call(uintptr(state.statusLabel), statusCommand)
	}
	if hwnd := state.initialFocusForPage(page); hwnd != 0 {
		procSettingsSetFocus.Call(uintptr(hwnd))
	}
	state.invalidateVisual()
}

func (state *windowsSettingsUI) initialFocusForPage(page int) windows.Handle {
	switch page {
	case settingsPageOverview:
		// Keep the default Overview screenshot visually clean. Keyboard users
		// still reach the first action with Tab through IsDialogMessage.
		return 0
	case settingsPageConnection:
		return state.serverEdit
	case settingsPageSync:
		return state.folderCombo
	case settingsPageBackup:
		return state.backupProfileCombo
	case settingsPageGeneral:
		return state.languageCombo
	default:
		return 0
	}
}

func (state *windowsSettingsUI) handlePageCommand(id, notify uint16) bool {
	if notify != settingsBNClicked && id != settingsIDBackupProfile {
		return false
	}
	switch id {
	case settingsIDNavOverview:
		state.showPage(settingsPageOverview)
	case settingsIDNavConnection:
		state.showPage(settingsPageConnection)
	case settingsIDNavSync:
		state.showPage(settingsPageSync)
	case settingsIDNavBackup:
		state.showPage(settingsPageBackup)
	case settingsIDNavGeneral:
		state.showPage(settingsPageGeneral)
	case settingsIDOpenLocal:
		state.openPrimaryLocalFolder()
	case settingsIDSaveSchedule:
		if state.selectedProfileID == "" {
			state.showError(state.tr("select_profile"))
		} else {
			state.saveProfile()
		}
	case settingsIDBackupProfile:
		if notify == settingsCBNSelect {
			state.selectBackupProfile()
		} else {
			return false
		}
	default:
		return false
	}
	return true
}

func (state *windowsSettingsUI) handlePageShortcut(message *settingsMessage) bool {
	if message == nil || message.Message != settingsWMKeyDown {
		return false
	}
	if message.WParam == settingsVKTab {
		ctrl, _, _ := procSettingsGetKeyState.Call(settingsVKControl)
		if uint16(ctrl)&0x8000 == 0 {
			return false
		}
		shift, _, _ := procSettingsGetKeyState.Call(settingsVKShift)
		delta := 1
		if uint16(shift)&0x8000 != 0 {
			delta = -1
		}
		state.cyclePage(delta)
		return true
	}

	if message.WParam != settingsVKUp && message.WParam != settingsVKDown &&
		message.WParam != settingsVKLeft && message.WParam != settingsVKRight {
		return false
	}
	focus, _, _ := procSettingsGetFocus.Call()
	page := -1
	for candidate, hwnd := range state.navButtons {
		if uintptr(hwnd) == focus {
			page = candidate
			break
		}
	}
	if page < 0 {
		return false
	}
	delta := 1
	if message.WParam == settingsVKUp || message.WParam == settingsVKLeft {
		delta = -1
	}
	page = (page + delta + settingsPageCount) % settingsPageCount
	state.showPage(page)
	procSettingsSetFocus.Call(uintptr(state.navButtons[page]))
	return true
}

func (state *windowsSettingsUI) cyclePage(delta int) {
	page := (state.page + delta) % settingsPageCount
	if page < 0 {
		page += settingsPageCount
	}
	state.showPage(page)
}

func (state *windowsSettingsUI) selectBackupProfile() {
	index := state.comboSelection(state.backupProfileCombo)
	if index < 0 || index >= len(state.profiles) {
		return
	}
	procSettingsSendMessage.Call(uintptr(state.profileList), settingsLBSetCurSel, uintptr(index), 0)
	state.applyProfile(state.profiles[index])
}

func (state *windowsSettingsUI) refreshBackupProfiles(selectID string) {
	if state.backupProfileCombo == 0 {
		return
	}
	procSettingsSendMessage.Call(uintptr(state.backupProfileCombo), settingsCBReset, 0, 0)
	selected := -1
	for i, profile := range state.profiles {
		label := filepath.Base(profile.Source)
		if strings.TrimSpace(label) == "" || label == "." {
			label = profile.Source
		}
		if !profile.Enabled {
			label += " — " + state.tr("profile_off")
		}
		state.comboAdd(state.backupProfileCombo, label)
		if profile.ID == selectID {
			selected = i
		}
	}
	if selected < 0 && len(state.profiles) > 0 {
		selected = 0
	}
	if selected >= 0 {
		procSettingsSendMessage.Call(uintptr(state.backupProfileCombo), settingsCBSetCurSel, uintptr(selected), 0)
	}
}

func (state *windowsSettingsUI) refreshOverview() {
	if state.overviewHeadline == 0 {
		return
	}
	enabled := 0
	failed := 0
	var lastSuccess time.Time
	var next time.Time
	for _, profile := range state.profiles {
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
		candidate := profile.NextDue()
		if !candidate.IsZero() && (next.IsZero() || candidate.Before(next)) {
			next = candidate.Local()
		}
	}

	switch {
	case len(state.profiles) == 0:
		state.setText(state.overviewHeadline, state.tr("overview_not_configured"))
		state.setText(state.overviewSubtitle, state.tr("overview_hint"))
	case failed > 0:
		state.setText(state.overviewHeadline, state.tr("overview_attention"))
		state.setText(state.overviewSubtitle, state.trf("overview_failed_count", failed))
	case enabled == 0:
		state.setText(state.overviewHeadline, state.tr("overview_paused"))
		state.setText(state.overviewSubtitle, state.tr("overview_no_enabled"))
	case lastSuccess.IsZero():
		state.setText(state.overviewHeadline, state.tr("overview_ready"))
		if state.connected {
			state.setText(state.overviewSubtitle, state.trf("overview_ready_connected", dashboardFolderCount(enabled, state.language)))
		} else {
			state.setText(state.overviewSubtitle, state.trf("overview_enabled_count", enabled))
		}
	default:
		state.setText(state.overviewHeadline, state.tr("overview_all_synced"))
		state.setText(state.overviewSubtitle, state.tr("overview_files_current"))
	}

	server := strings.TrimSpace(state.text(state.serverEdit))
	if server == "" {
		state.setText(state.overviewConnection, state.tr("overview_connection_none"))
		state.setText(state.overviewConnectionHint, "")
	} else if state.connected {
		state.setText(state.overviewConnection, state.tr("overview_connection_connected"))
		state.setText(state.overviewConnectionHint, dashboardServerLabel(server))
	} else {
		state.setText(state.overviewConnection, state.tr("overview_connection_configured"))
		state.setText(state.overviewConnectionHint, dashboardServerLabel(server))
	}

	if folder := state.primaryLocalFolder(); folder != "" {
		label := filepath.Base(folder)
		if label == "." || label == string(filepath.Separator) || strings.TrimSpace(label) == "" {
			label = folder
		}
		state.setText(state.overviewSync, label)
		count := enabled
		if count == 0 {
			count = len(state.profiles)
		}
		state.setText(state.overviewSyncHint, dashboardFolderCount(count, state.language))
	} else {
		state.setText(state.overviewSync, state.tr("overview_folder_none"))
		state.setText(state.overviewSyncHint, "")
	}

	if lastSuccess.IsZero() {
		state.setText(state.overviewLast, state.tr("overview_not_synced"))
	} else {
		state.setText(state.overviewLast, dashboardTime(lastSuccess, state.language))
	}
	if next.IsZero() {
		state.setText(state.overviewNext, state.tr("overview_next_manual_short"))
	} else {
		state.setText(state.overviewNext, dashboardTime(next, state.language))
	}

	autostart, err := windowsclient.UserAgentAutostartStatus()
	if err != nil {
		state.setText(state.overviewBackup, state.tr("overview_agent_unknown"))
		state.setVisualRole(state.overviewBackup, settingsVisualCardMuted)
	} else if autostart.Enabled {
		state.setText(state.overviewBackup, state.tr("backup_on"))
		state.setVisualRole(state.overviewBackup, settingsVisualCardPositive)
	} else {
		state.setText(state.overviewBackup, state.tr("backup_off"))
		state.setVisualRole(state.overviewBackup, settingsVisualCardDanger)
	}

	state.refreshRecentActivity()
	state.refreshStorageSummary()
	state.invalidateVisual()
}

func (state *windowsSettingsUI) refreshRecentActivity() {
	profiles := append([]windowsclient.SyncProfile(nil), state.profiles...)
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
	row := 0
	for _, profile := range profiles {
		if row >= len(state.overviewRecent) {
			break
		}
		if profile.LastAttemptAt == nil {
			continue
		}
		name := filepath.Base(profile.Source)
		if strings.TrimSpace(name) == "" || name == "." {
			name = profile.Source
		}
		result := state.tr("recent_activity_ok")
		if profile.LastError != "" {
			result = state.tr("recent_activity_error")
		}
		state.setText(state.overviewRecent[row], state.trf("recent_activity_row", name, result, dashboardTime(profile.LastAttemptAt.Local(), state.language)))
		row++
	}
	if row == 0 {
		state.setText(state.overviewRecent[0], state.tr("recent_activity_empty"))
		row = 1
	}
	for ; row < len(state.overviewRecent); row++ {
		state.setText(state.overviewRecent[row], "")
	}
}

func (state *windowsSettingsUI) refreshStorageSummary() {
	var used, quota int64
	known := false
	for _, folder := range state.folders {
		if !folder.CanRead || !folder.UsageKnown {
			continue
		}
		known = true
		used += folder.UsedBytes + folder.ReservedBytes
		if folder.QuotaBytes > 0 {
			quota += folder.QuotaBytes
		}
	}
	state.overviewStoragePercent = 0
	if !known {
		state.setText(state.overviewStorage, state.tr("storage_unknown"))
		hint := state.tr("storage_connect_hint")
		if state.connected {
			hint = state.tr("storage_server_hint")
		}
		state.setText(state.overviewStorageHint, hint)
		return
	}
	if quota > 0 {
		state.setText(state.overviewStorage, state.trf("storage_used_of", dashboardBytes(used), dashboardBytes(quota)))
		free := quota - used
		if free < 0 {
			free = 0
		}
		state.setText(state.overviewStorageHint, state.trf("storage_available", dashboardBytes(free)))
		state.overviewStoragePercent = int((used * 100) / quota)
		return
	}
	state.setText(state.overviewStorage, dashboardBytes(used))
	state.setText(state.overviewStorageHint, state.tr("storage_unlimited"))
}

func dashboardServerLabel(server string) string {
	server = strings.TrimSpace(server)
	server = strings.TrimPrefix(server, "https://")
	server = strings.TrimPrefix(server, "http://")
	server = strings.TrimRight(server, "/")
	if len([]rune(server)) > 24 {
		runes := []rune(server)
		server = string(runes[:23]) + "…"
	}
	return server
}

func dashboardFolderCount(count int, language string) string {
	if normalizeUILanguage(language) != uiLanguageRussian {
		if count == 1 {
			return "1 folder"
		}
		return fmt.Sprintf("%d folders", count)
	}
	mod10 := count % 10
	mod100 := count % 100
	switch {
	case mod10 == 1 && mod100 != 11:
		return fmt.Sprintf("%d папка", count)
	case mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14):
		return fmt.Sprintf("%d папки", count)
	default:
		return fmt.Sprintf("%d папок", count)
	}
}

func dashboardBytes(value int64) string {
	if value < 0 {
		value = 0
	}
	const unit = int64(1024)
	if value < unit {
		return fmt.Sprintf("%d B", value)
	}
	div, exp := unit, 0
	for n := value / unit; n >= unit && exp < 4; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(value)/float64(div), "KMGTPE"[exp])
}

func dashboardTime(value time.Time, language string) string {
	value = value.Local()
	now := time.Now()
	if value.Year() == now.Year() && value.YearDay() == now.YearDay() {
		if normalizeUILanguage(language) == uiLanguageRussian {
			return "Сегодня, " + value.Format("15:04")
		}
		return "Today, " + value.Format("15:04")
	}
	return value.Format("02.01 15:04")
}

func (state *windowsSettingsUI) primaryLocalFolder() string {
	profiles := state.profiles
	if state.selectedProfileID != "" {
		for _, profile := range profiles {
			if profile.ID == state.selectedProfileID {
				return localFolderForProfile(profile)
			}
		}
	}
	for _, profile := range profiles {
		if profile.Enabled {
			return localFolderForProfile(profile)
		}
	}
	if len(profiles) > 0 {
		return localFolderForProfile(profiles[0])
	}
	return ""
}

func localFolderForProfile(profile windowsclient.SyncProfile) string {
	path := strings.TrimSpace(profile.Source)
	if path == "" {
		return ""
	}
	info, err := os.Stat(path)
	if err == nil && !info.IsDir() {
		return filepath.Dir(path)
	}
	return path
}

func primaryLocalFolderForConfig(configPath string) string {
	profiles, err := windowsclient.LoadSyncProfiles(configPath)
	if err != nil {
		return ""
	}
	for _, profile := range profiles {
		if profile.Enabled {
			return localFolderForProfile(profile)
		}
	}
	if len(profiles) > 0 {
		return localFolderForProfile(profiles[0])
	}
	return ""
}

func (state *windowsSettingsUI) openPrimaryLocalFolder() {
	path := state.primaryLocalFolder()
	if path == "" {
		state.showError(state.tr("open_folder_unavailable"))
		return
	}
	command := exec.Command("explorer.exe", path)
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: settingsCreateNoWindow}
	if err := command.Start(); err != nil {
		state.showTechnicalError(fmt.Errorf("open local folder: %w", err))
		return
	}
	_ = command.Process.Release()
}
