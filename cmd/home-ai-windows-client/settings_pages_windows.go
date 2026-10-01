//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

	static := func(name, key string, x, y, w, h int32, page int) windows.Handle {
		hwnd := state.createControl(module, "STATIC", state.tr(key), settingsWSChild|settingsWSVisible, x, y, w, h, 0, windows.Handle(font))
		if name != "" {
			state.localized[name] = hwnd
		}
		if page >= 0 {
			state.trackPage(page, hwnd)
		}
		return hwnd
	}
	edit := func(id uint16, text string, x, y, w, h int32, extra uint32, page int) windows.Handle {
		hwnd := state.createControl(module, "EDIT", text, settingsWSChild|settingsWSVisible|settingsWSTabStop|settingsWSBorder|settingsESAutoHScroll|extra, x, y, w, h, id, windows.Handle(font))
		state.trackPage(page, hwnd)
		return hwnd
	}
	button := func(name string, id uint16, key string, x, y, w, h int32, page int) windows.Handle {
		hwnd := state.createControl(module, "BUTTON", state.tr(key), settingsWSChild|settingsWSVisible|settingsWSTabStop, x, y, w, h, id, windows.Handle(font))
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

	// Brand and persistent left navigation.
	if state.windowIcon != 0 {
		iconView := state.createControl(module, "STATIC", "", settingsWSChild|settingsWSVisible|settingsSSIcon, 22, 18, 38, 38, 0, windows.Handle(font))
		if iconView != 0 {
			procSettingsSendMessage.Call(uintptr(iconView), settingsSTMSetIcon, uintptr(state.windowIcon), 0)
		}
	}
	static("brand_name", "brand_name", 70, 24, 120, 28, -1)

	nav := func(page int, id uint16, key string, y int32, first bool) {
		style := uint32(settingsWSChild | settingsWSVisible | settingsWSTabStop | settingsBSAutoRadio | settingsBSPushLike)
		if first {
			style |= settingsWSGroup
		}
		hwnd := state.createControl(module, "BUTTON", state.tr(key), style, 20, y, 168, 38, id, windows.Handle(font))
		state.navButtons[page] = hwnd
		state.localized[fmt.Sprintf("nav_%d", page)] = hwnd
	}
	nav(settingsPageOverview, settingsIDNavOverview, "nav_overview", 82, true)
	nav(settingsPageConnection, settingsIDNavConnection, "nav_connection", 126, false)
	nav(settingsPageSync, settingsIDNavSync, "nav_sync", 170, false)
	nav(settingsPageBackup, settingsIDNavBackup, "nav_backup", 214, false)
	nav(settingsPageGeneral, settingsIDNavGeneral, "nav_settings", 258, false)

	// Overview.
	static("overview_title", "nav_overview", 226, 30, 300, 30, settingsPageOverview)
	state.overviewHeadline = static("", "overview_not_configured", 226, 82, 660, 34, settingsPageOverview)
	state.overviewSubtitle = static("", "overview_hint", 226, 118, 660, 24, settingsPageOverview)
	button("overview_sync_button", settingsIDSyncNow, "sync_now", 690, 74, 210, 36, settingsPageOverview)
	button("overview_open_button", settingsIDOpenLocal, "open_home_folder", 690, 116, 210, 34, settingsPageOverview)

	static("overview_connection_title", "overview_connection", 226, 184, 210, 24, settingsPageOverview)
	state.overviewConnection = static("", "overview_not_configured", 226, 212, 300, 50, settingsPageOverview)
	static("overview_sync_title", "overview_sync_state", 548, 184, 210, 24, settingsPageOverview)
	state.overviewSync = static("", "overview_not_configured", 548, 212, 352, 50, settingsPageOverview)

	static("overview_backup_title", "overview_backup_state", 226, 294, 210, 24, settingsPageOverview)
	state.overviewBackup = static("", "overview_not_configured", 226, 322, 300, 64, settingsPageOverview)
	static("overview_next_title", "overview_next", 548, 294, 210, 24, settingsPageOverview)
	state.overviewNext = static("", "overview_not_configured", 548, 322, 352, 64, settingsPageOverview)

	static("overview_last_title", "overview_last", 226, 426, 210, 24, settingsPageOverview)
	state.overviewLast = static("", "overview_not_synced", 226, 454, 674, 82, settingsPageOverview)

	// Connection.
	static("connection_title", "nav_connection", 226, 30, 300, 30, settingsPageConnection)
	static("connection_hint", "connection_hint", 226, 68, 650, 38, settingsPageConnection)
	static("server_label", "server", 226, 132, 120, 22, settingsPageConnection)
	state.serverEdit = edit(settingsIDServerEdit, "", 226, 156, 510, 28, 0, settingsPageConnection)
	static("user_label", "user", 226, 204, 120, 22, settingsPageConnection)
	state.usernameEdit = edit(settingsIDUsernameEdit, "", 226, 228, 250, 28, 0, settingsPageConnection)
	static("password_label", "password", 500, 204, 120, 22, settingsPageConnection)
	state.passwordEdit = edit(settingsIDPasswordEdit, "", 500, 228, 236, 28, settingsESPassword, settingsPageConnection)
	static("password_hint", "password_hint", 226, 266, 510, 22, settingsPageConnection)
	button("connect_button", settingsIDConnect, "connect_save", 226, 310, 220, 36, settingsPageConnection)

	// Synchronization.
	static("sync_title", "nav_sync", 226, 30, 300, 30, settingsPageSync)
	static("sync_hint", "sync_hint", 226, 68, 650, 38, settingsPageSync)
	static("remote_label", "remote_folder", 226, 118, 170, 22, settingsPageSync)
	state.folderCombo = combo(settingsIDFolderCombo, 226, 142, 674, 240, settingsPageSync)
	static("local_label", "local_source", 226, 190, 170, 22, settingsPageSync)
	state.sourceEdit = edit(settingsIDSourceEdit, "", 226, 214, 574, 28, 0, settingsPageSync)
	button("browse_button", settingsIDBrowse, "browse", 812, 213, 88, 30, settingsPageSync)
	static("destination_label", "destination", 226, 258, 170, 22, settingsPageSync)
	state.destination = edit(settingsIDDestination, "", 226, 282, 674, 28, 0, settingsPageSync)

	state.saveProfileBtn = button("save_button", settingsIDSaveProfile, "add_profile", 226, 326, 140, 32, settingsPageSync)
	button("new_button", settingsIDNewProfile, "new_clear", 376, 326, 118, 32, settingsPageSync)
	button("enable_button", settingsIDEnableProfile, "enable", 504, 326, 100, 32, settingsPageSync)
	button("disable_button", settingsIDDisableProfile, "disable", 614, 326, 100, 32, settingsPageSync)
	button("delete_button", settingsIDDeleteProfile, "delete", 724, 326, 84, 32, settingsPageSync)
	button("refresh_button", settingsIDRefresh, "refresh", 818, 326, 82, 32, settingsPageSync)

	static("profiles_label", "sync_profiles", 226, 378, 260, 22, settingsPageSync)
	state.profileList = state.createControl(module, "LISTBOX", "", settingsWSChild|settingsWSVisible|settingsWSTabStop|settingsWSBorder|settingsWSVScroll|settingsLBSNotify, 226, 404, 674, 142, settingsIDProfileList, windows.Handle(font))
	state.trackPage(settingsPageSync, state.profileList)
	button("sync_button", settingsIDSyncNow, "sync_now", 690, 560, 210, 34, settingsPageSync)

	// Backups / schedule.
	static("backup_title", "nav_backup", 226, 30, 300, 30, settingsPageBackup)
	static("backup_hint", "backup_hint", 226, 68, 650, 44, settingsPageBackup)
	static("backup_profile_label", "backup_folder", 226, 132, 190, 22, settingsPageBackup)
	state.backupProfileCombo = combo(settingsIDBackupProfile, 226, 156, 674, 220, settingsPageBackup)
	static("interval_label", "interval", 226, 210, 180, 22, settingsPageBackup)
	state.interval = edit(settingsIDInterval, "15m", 226, 234, 220, 28, 0, settingsPageBackup)
	static("conflict_label", "conflict", 476, 210, 180, 22, settingsPageBackup)
	state.conflict = combo(settingsIDConflict, 476, 234, 300, 160, settingsPageBackup)
	state.comboAdd(state.conflict, state.tr("conflict_stop"))
	state.comboAdd(state.conflict, state.tr("conflict_skip"))
	state.comboAdd(state.conflict, state.tr("conflict_replace"))
	procSettingsSendMessage.Call(uintptr(state.conflict), settingsCBSetCurSel, 0, 0)
	button("schedule_save_button", settingsIDSaveSchedule, "save_schedule", 226, 286, 220, 34, settingsPageBackup)

	static("backup_agent_title", "backup_agent_title", 226, 356, 230, 24, settingsPageBackup)
	state.agentLabel = static("", "autostart_checking", 226, 386, 674, 42, settingsPageBackup)
	button("agent_enable", settingsIDAgentEnable, "enable_agent", 226, 446, 180, 34, settingsPageBackup)
	button("agent_disable", settingsIDAgentDisable, "disable_agent", 418, 446, 180, 34, settingsPageBackup)

	// General settings.
	static("general_title", "nav_settings", 226, 30, 300, 30, settingsPageGeneral)
	static("general_hint", "general_hint", 226, 68, 650, 38, settingsPageGeneral)
	static("language_label", "language", 226, 132, 180, 22, settingsPageGeneral)
	state.languageCombo = combo(settingsIDLanguage, 226, 156, 280, 120, settingsPageGeneral)
	state.comboAdd(state.languageCombo, "Русский")
	state.comboAdd(state.languageCombo, "English")
	if state.language == uiLanguageRussian {
		procSettingsSendMessage.Call(uintptr(state.languageCombo), settingsCBSetCurSel, 0, 0)
	} else {
		procSettingsSendMessage.Call(uintptr(state.languageCombo), settingsCBSetCurSel, 1, 0)
	}
	static("general_background_title", "general_background_title", 226, 228, 240, 24, settingsPageGeneral)
	static("general_background_hint", "general_background_hint", 226, 258, 650, 70, settingsPageGeneral)

	// A compact status line stays available on every page.
	static("status_title", "status", 226, 610, 78, 22, -1)
	state.statusLabel = state.createControl(module, "STATIC", state.tr("ready"), settingsWSChild|settingsWSVisible, 304, 610, 596, 42, 0, windows.Handle(font))

	if state.serverEdit == 0 || state.usernameEdit == 0 || state.passwordEdit == 0 ||
		state.folderCombo == 0 || state.sourceEdit == 0 || state.destination == 0 ||
		state.interval == 0 || state.conflict == 0 || state.profileList == 0 ||
		state.backupProfileCombo == 0 || state.statusLabel == 0 || state.agentLabel == 0 ||
		state.languageCombo == 0 {
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
		state.setText(state.overviewSubtitle, state.trf("overview_enabled_count", enabled))
	}

	server := strings.TrimSpace(state.text(state.serverEdit))
	user := strings.TrimSpace(state.text(state.usernameEdit))
	if server == "" {
		state.setText(state.overviewConnection, state.tr("overview_not_configured"))
	} else if state.connected {
		state.setText(state.overviewConnection, state.trf("overview_connected_to", server, user))
	} else {
		state.setText(state.overviewConnection, state.trf("overview_configured_server", server, user))
	}

	state.setText(state.overviewSync, state.trf("overview_sync_count", enabled, len(state.profiles)))
	autostart, err := windowsclient.UserAgentAutostartStatus()
	if err != nil {
		state.setText(state.overviewBackup, state.tr("overview_agent_unknown"))
	} else if autostart.Enabled {
		state.setText(state.overviewBackup, state.tr("overview_agent_on"))
	} else {
		state.setText(state.overviewBackup, state.tr("overview_agent_off"))
	}
	if next.IsZero() {
		state.setText(state.overviewNext, state.tr("overview_next_manual"))
	} else {
		state.setText(state.overviewNext, next.Format("02.01.2006 15:04"))
	}
	if lastSuccess.IsZero() {
		state.setText(state.overviewLast, state.tr("overview_not_synced"))
	} else {
		state.setText(state.overviewLast, state.trf("overview_last_success", lastSuccess.Format("02.01.2006 15:04")))
	}
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
