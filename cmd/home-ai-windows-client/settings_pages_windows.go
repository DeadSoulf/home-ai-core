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
)

var (
	procSettingsIsDialogMessage = settingsUser32.NewProc("IsDialogMessageW")
	procSettingsGetKeyState     = settingsUser32.NewProc("GetKeyState")
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
		hwnd := state.createControl(module, "BUTTON", state.tr(key), settingsWSChild|settingsWSVisible|settingsWSTabStop|settingsBSFlat, x, y, w, h, id, windows.Handle(font))
		if name != "" {
			state.localized[name] = hwnd
		}
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

	// Persistent HOME AI brand and cloud-drive-style left navigation.
	if state.windowIcon != 0 {
		iconView := state.createControl(module, "STATIC", "", settingsWSChild|settingsWSVisible|settingsSSIcon, 22, 20, 40, 40, 0, windows.Handle(font))
		if iconView != 0 {
			state.setVisualRole(iconView, settingsVisualSidebar)
			procSettingsSendMessage.Call(uintptr(iconView), settingsSTMSetIcon, uintptr(state.windowIcon), 0)
		}
	}
	brand := static("brand_name", "brand_name", 72, 27, 110, 28, -1, settingsVisualSidebar)
	state.setControlFont(brand, state.visual.titleFont)

	nav := func(page int, id uint16, key string, y int32, first bool) {
		style := uint32(settingsWSChild | settingsWSVisible | settingsWSTabStop | settingsBSAutoRadio | settingsBSPushLike | settingsBSFlat)
		if first {
			style |= settingsWSGroup
		}
		hwnd := state.createControl(module, "BUTTON", state.tr(key), style, 18, y, 170, 38, id, windows.Handle(font))
		state.navButtons[page] = hwnd
		state.localized[fmt.Sprintf("nav_%d", page)] = hwnd
	}
	nav(settingsPageOverview, settingsIDNavOverview, "nav_overview", 92, true)
	nav(settingsPageConnection, settingsIDNavConnection, "nav_connection", 136, false)
	nav(settingsPageSync, settingsIDNavSync, "nav_sync", 180, false)
	nav(settingsPageBackup, settingsIDNavBackup, "nav_backup", 224, false)
	nav(settingsPageGeneral, settingsIDNavGeneral, "nav_settings", 268, false)
	staticText("", "© 2026 TexNik", 18, 692, 170, 22, -1, settingsVisualSidebar)

	// Overview — visually mirrors the approved HOME AI dashboard mockup.
	title := static("overview_title", "nav_overview", 225, 24, 360, 30, settingsPageOverview, settingsVisualMain)
	state.setControlFont(title, state.visual.titleFont)

	heroIcon := staticText("", "✓", 250, 94, 42, 42, settingsPageOverview, settingsVisualStatusIcon)
	state.setControlFont(heroIcon, state.visual.headlineFont)
	state.overviewHeadline = static("", "overview_not_configured", 305, 84, 555, 38, settingsPageOverview, settingsVisualHero)
	state.setControlFont(state.overviewHeadline, state.visual.headlineFont)
	state.overviewSubtitle = static("", "overview_hint", 305, 126, 555, 26, settingsPageOverview, settingsVisualHero)
	state.setControlFont(state.overviewSubtitle, state.visual.subtitleFont)
	button("overview_sync_button", settingsIDSyncNow, "sync_now", 906, 88, 218, 38, settingsPageOverview)
	button("overview_open_button", settingsIDOpenLocal, "open_home_folder", 906, 136, 218, 32, settingsPageOverview)

	connTitle := static("overview_connection_title", "overview_connection", 245, 228, 180, 22, settingsPageOverview, settingsVisualCard)
	state.setControlFont(connTitle, state.visual.cardTitleFont)
	state.overviewConnection = staticText("", state.tr("overview_not_configured"), 245, 254, 178, 42, settingsPageOverview, settingsVisualCard)
	state.setControlFont(state.overviewConnection, state.visual.cardValueFont)

	folderTitle := static("overview_folder_title", "overview_folder", 476, 228, 180, 22, settingsPageOverview, settingsVisualCard)
	state.setControlFont(folderTitle, state.visual.cardTitleFont)
	state.overviewSync = staticText("", state.tr("overview_not_configured"), 476, 254, 178, 42, settingsPageOverview, settingsVisualCard)
	state.setControlFont(state.overviewSync, state.visual.cardValueFont)

	lastTitle := static("overview_last_title", "overview_last", 707, 228, 180, 22, settingsPageOverview, settingsVisualCard)
	state.setControlFont(lastTitle, state.visual.cardTitleFont)
	state.overviewLast = staticText("", state.tr("overview_not_synced"), 707, 254, 178, 42, settingsPageOverview, settingsVisualCard)
	state.setControlFont(state.overviewLast, state.visual.cardValueFont)

	nextTitle := static("overview_next_title", "overview_next", 938, 228, 190, 22, settingsPageOverview, settingsVisualCard)
	state.setControlFont(nextTitle, state.visual.cardTitleFont)
	state.overviewNext = staticText("", state.tr("overview_next_manual"), 938, 254, 190, 42, settingsPageOverview, settingsVisualCard)
	state.setControlFont(state.overviewNext, state.visual.cardValueFont)

	recentTitle := static("overview_recent_title", "recent_activity", 245, 350, 260, 26, settingsPageOverview, settingsVisualCard)
	state.setControlFont(recentTitle, state.visual.titleFont)
	for i := range state.overviewRecent {
		state.overviewRecent[i] = staticText("", state.tr("recent_activity_empty"), 250, int32(392+i*58), 520, 42, settingsPageOverview, settingsVisualCard)
	}

	backupTitle := static("overview_backup_title", "backup_summary", 835, 350, 230, 24, settingsPageOverview, settingsVisualCard)
	state.setControlFont(backupTitle, state.visual.cardTitleFont)
	state.overviewBackup = staticText("", state.tr("overview_agent_unknown"), 835, 380, 285, 52, settingsPageOverview, settingsVisualCard)
	state.setControlFont(state.overviewBackup, state.visual.cardValueFont)

	storageTitle := static("overview_storage_title", "storage_summary", 835, 490, 240, 24, settingsPageOverview, settingsVisualCard)
	state.setControlFont(storageTitle, state.visual.cardTitleFont)
	state.overviewStorage = staticText("", state.tr("storage_unknown"), 835, 518, 285, 28, settingsPageOverview, settingsVisualCard)
	state.setControlFont(state.overviewStorage, state.visual.cardValueFont)
	state.overviewStorageHint = staticText("", "", 835, 568, 285, 20, settingsPageOverview, settingsVisualCard)

	// Connection.
	connectionTitle := static("connection_title", "nav_connection", 250, 42, 320, 30, settingsPageConnection, settingsVisualCard)
	state.setControlFont(connectionTitle, state.visual.titleFont)
	static("connection_hint", "connection_hint", 250, 82, 760, 38, settingsPageConnection, settingsVisualCard)
	static("server_label", "server", 250, 146, 120, 22, settingsPageConnection, settingsVisualCard)
	state.serverEdit = edit(settingsIDServerEdit, "", 250, 172, 570, 30, 0, settingsPageConnection)
	static("user_label", "user", 250, 224, 120, 22, settingsPageConnection, settingsVisualCard)
	state.usernameEdit = edit(settingsIDUsernameEdit, "", 250, 250, 270, 30, 0, settingsPageConnection)
	static("password_label", "password", 548, 224, 120, 22, settingsPageConnection, settingsVisualCard)
	state.passwordEdit = edit(settingsIDPasswordEdit, "", 548, 250, 272, 30, settingsESPassword, settingsPageConnection)
	static("password_hint", "password_hint", 250, 292, 570, 22, settingsPageConnection, settingsVisualCard)
	button("connect_button", settingsIDConnect, "connect_save", 250, 342, 230, 38, settingsPageConnection)

	// Synchronization.
	syncTitle := static("sync_title", "nav_sync", 250, 42, 340, 30, settingsPageSync, settingsVisualCard)
	state.setControlFont(syncTitle, state.visual.titleFont)
	static("sync_hint", "sync_hint", 250, 82, 780, 38, settingsPageSync, settingsVisualCard)
	static("remote_label", "remote_folder", 250, 136, 180, 22, settingsPageSync, settingsVisualCard)
	state.folderCombo = combo(settingsIDFolderCombo, 250, 162, 760, 240, settingsPageSync)
	static("local_label", "local_source", 250, 212, 180, 22, settingsPageSync, settingsVisualCard)
	state.sourceEdit = edit(settingsIDSourceEdit, "", 250, 238, 646, 30, 0, settingsPageSync)
	button("browse_button", settingsIDBrowse, "browse", 910, 237, 100, 32, settingsPageSync)
	static("destination_label", "destination", 250, 286, 180, 22, settingsPageSync, settingsVisualCard)
	state.destination = edit(settingsIDDestination, "", 250, 312, 760, 30, 0, settingsPageSync)

	state.saveProfileBtn = button("save_button", settingsIDSaveProfile, "add_profile", 250, 360, 150, 34, settingsPageSync)
	button("new_button", settingsIDNewProfile, "new_clear", 410, 360, 132, 34, settingsPageSync)
	button("enable_button", settingsIDEnableProfile, "enable", 552, 360, 105, 34, settingsPageSync)
	button("disable_button", settingsIDDisableProfile, "disable", 667, 360, 105, 34, settingsPageSync)
	button("delete_button", settingsIDDeleteProfile, "delete", 782, 360, 100, 34, settingsPageSync)
	button("refresh_button", settingsIDRefresh, "refresh", 892, 360, 118, 34, settingsPageSync)

	static("profiles_label", "sync_profiles", 250, 418, 300, 22, settingsPageSync, settingsVisualCard)
	state.profileList = state.createControl(module, "LISTBOX", "", settingsWSChild|settingsWSVisible|settingsWSTabStop|settingsWSBorder|settingsWSVScroll|settingsLBSNotify, 250, 446, 760, 150, settingsIDProfileList, windows.Handle(font))
	state.trackPage(settingsPageSync, state.profileList)
	button("sync_button", settingsIDSyncNow, "sync_now", 790, 612, 220, 36, settingsPageSync)

	// Backups / schedule.
	backupPageTitle := static("backup_title", "nav_backup", 250, 42, 340, 30, settingsPageBackup, settingsVisualCard)
	state.setControlFont(backupPageTitle, state.visual.titleFont)
	static("backup_hint", "backup_hint", 250, 82, 780, 44, settingsPageBackup, settingsVisualCard)
	static("backup_profile_label", "backup_folder", 250, 146, 190, 22, settingsPageBackup, settingsVisualCard)
	state.backupProfileCombo = combo(settingsIDBackupProfile, 250, 172, 760, 220, settingsPageBackup)
	static("interval_label", "interval", 250, 228, 180, 22, settingsPageBackup, settingsVisualCard)
	state.interval = edit(settingsIDInterval, "15m", 250, 254, 240, 30, 0, settingsPageBackup)
	static("conflict_label", "conflict", 530, 228, 180, 22, settingsPageBackup, settingsVisualCard)
	state.conflict = combo(settingsIDConflict, 530, 254, 330, 160, settingsPageBackup)
	state.comboAdd(state.conflict, state.tr("conflict_stop"))
	state.comboAdd(state.conflict, state.tr("conflict_skip"))
	state.comboAdd(state.conflict, state.tr("conflict_replace"))
	procSettingsSendMessage.Call(uintptr(state.conflict), settingsCBSetCurSel, 0, 0)
	button("schedule_save_button", settingsIDSaveSchedule, "save_schedule", 250, 310, 230, 36, settingsPageBackup)

	static("backup_agent_title", "backup_agent_title", 250, 390, 260, 24, settingsPageBackup, settingsVisualCard)
	state.agentLabel = static("", "autostart_checking", 250, 422, 760, 48, settingsPageBackup, settingsVisualCard)
	button("agent_enable", settingsIDAgentEnable, "enable_agent", 250, 492, 190, 36, settingsPageBackup)
	button("agent_disable", settingsIDAgentDisable, "disable_agent", 452, 492, 190, 36, settingsPageBackup)

	// General settings.
	generalTitle := static("general_title", "nav_settings", 250, 42, 340, 30, settingsPageGeneral, settingsVisualCard)
	state.setControlFont(generalTitle, state.visual.titleFont)
	static("general_hint", "general_hint", 250, 82, 780, 38, settingsPageGeneral, settingsVisualCard)
	static("language_label", "language", 250, 146, 180, 22, settingsPageGeneral, settingsVisualCard)
	state.languageCombo = combo(settingsIDLanguage, 250, 172, 300, 120, settingsPageGeneral)
	state.comboAdd(state.languageCombo, "Русский")
	state.comboAdd(state.languageCombo, "English")
	if state.language == uiLanguageRussian {
		procSettingsSendMessage.Call(uintptr(state.languageCombo), settingsCBSetCurSel, 0, 0)
	} else {
		procSettingsSendMessage.Call(uintptr(state.languageCombo), settingsCBSetCurSel, 1, 0)
	}
	static("general_background_title", "general_background_title", 250, 250, 260, 24, settingsPageGeneral, settingsVisualCard)
	static("general_background_hint", "general_background_hint", 250, 282, 760, 70, settingsPageGeneral, settingsVisualCard)

	// Small global status line.
	static("status_title", "status", 225, 692, 78, 22, -1, settingsVisualMain)
	state.statusLabel = state.createControl(module, "STATIC", state.tr("ready"), settingsWSChild|settingsWSVisible, 305, 692, 810, 30, 0, windows.Handle(font))
	state.setVisualRole(state.statusLabel, settingsVisualMain)

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
	if page == settingsPageOverview {
		state.refreshOverview()
	}
	if hwnd := state.initialFocusForPage(page); hwnd != 0 {
		procSettingsSetFocus.Call(uintptr(hwnd))
	}
	state.invalidateVisual()
}

func (state *windowsSettingsUI) initialFocusForPage(page int) windows.Handle {
	switch page {
	case settingsPageOverview:
		return state.localized["overview_sync_button"]
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
	if message == nil || message.Message != settingsWMKeyDown || message.WParam != settingsVKTab {
		return false
	}
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
		state.setText(state.overviewSubtitle, state.trf("overview_enabled_count", enabled))
	default:
		state.setText(state.overviewHeadline, state.tr("overview_all_synced"))
		state.setText(state.overviewSubtitle, state.tr("overview_files_current"))
	}

	server := strings.TrimSpace(state.text(state.serverEdit))
	user := strings.TrimSpace(state.text(state.usernameEdit))
	if server == "" {
		state.setText(state.overviewConnection, state.tr("overview_not_configured"))
	} else if state.connected {
		state.setText(state.overviewConnection, state.trf("overview_connected_short", server))
	} else {
		state.setText(state.overviewConnection, state.trf("overview_configured_short", server, user))
	}

	if folder := state.primaryLocalFolder(); folder != "" {
		label := filepath.Base(folder)
		if label == "." || label == string(filepath.Separator) || strings.TrimSpace(label) == "" {
			label = folder
		}
		state.setText(state.overviewSync, label)
	} else {
		state.setText(state.overviewSync, state.tr("overview_folder_none"))
	}

	if lastSuccess.IsZero() {
		state.setText(state.overviewLast, state.tr("overview_not_synced"))
	} else {
		state.setText(state.overviewLast, dashboardTime(lastSuccess, state.language))
	}
	if next.IsZero() {
		state.setText(state.overviewNext, state.tr("overview_next_manual"))
	} else {
		state.setText(state.overviewNext, dashboardTime(next, state.language))
	}

	autostart, err := windowsclient.UserAgentAutostartStatus()
	if err != nil {
		state.setText(state.overviewBackup, state.tr("overview_agent_unknown"))
	} else if autostart.Enabled {
		state.setText(state.overviewBackup, state.trf("overview_agent_on_count", enabled))
	} else {
		state.setText(state.overviewBackup, state.tr("overview_agent_off"))
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
	for ; row < len(state.overviewRecent); row++ {
		state.setText(state.overviewRecent[row], state.tr("recent_activity_empty"))
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
		state.setText(state.overviewStorageHint, state.tr("storage_connect_hint"))
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
