//go:build windows

package main

import (
	"fmt"
	"unsafe"

	"github.com/DeadSoulf/home-ai-core/internal/windowsclient"
	"golang.org/x/sys/windows"
)

func (state *windowsSettingsUI) tr(key string) string {
	return textForLanguage(state.language, key)
}

func (state *windowsSettingsUI) trf(key string, args ...any) string {
	return textForLanguagef(state.language, key, args...)
}

func (state *windowsSettingsUI) applyLanguageTexts() {
	state.setText(state.hwnd, state.tr("window_title"))
	for _, item := range [][2]string{
		{"brand_name", "brand_name"},
		{"nav_0", "nav_overview"},
		{"nav_1", "nav_connection"},
		{"nav_2", "nav_sync"},
		{"nav_3", "nav_backup"},
		{"nav_4", "nav_settings"},
		{"overview_title", "nav_overview"},
		{"overview_main_subtitle", "overview_main_subtitle"},
		{"overview_sync_button", "sync_compact"},
		{"overview_open_button", "open_home_folder"},
		{"overview_connection_title", "overview_connection"},
		{"overview_folder_title", "overview_folder"},
		{"overview_backup_title", "backup_summary"},
		{"overview_next_title", "overview_next"},
		{"overview_last_title", "overview_last_short"},
		{"overview_recent_title", "recent_activity"},
		{"overview_recent_hint", "recent_activity_hint"},
		{"overview_backup_label", "backup_background"},
		{"overview_storage_title", "storage_summary"},
		{"connection_title", "nav_connection"},
		{"connection_hint", "connection_hint"},
		{"server_label", "server"},
		{"connect_button", "connect_save"},
		{"user_label", "user"},
		{"password_label", "password"},
		{"password_hint", "password_hint"},
		{"sync_title", "nav_sync"},
		{"sync_hint", "sync_hint"},
		{"remote_label", "remote_folder"},
		{"local_label", "local_source"},
		{"browse_button", "browse"},
		{"destination_label", "destination"},
		{"new_button", "new_clear"},
		{"enable_button", "enable"},
		{"disable_button", "disable"},
		{"delete_button", "delete"},
		{"refresh_button", "refresh"},
		{"profiles_label", "sync_profiles"},
		{"sync_button", "sync_now"},
		{"backup_title", "nav_backup"},
		{"backup_hint", "backup_hint"},
		{"backup_profile_label", "backup_folder"},
		{"interval_label", "interval"},
		{"conflict_label", "conflict"},
		{"schedule_save_button", "save_schedule"},
		{"backup_agent_title", "backup_agent_title"},
		{"agent_enable", "enable_agent"},
		{"agent_disable", "disable_agent"},
		{"general_title", "nav_settings"},
		{"general_hint", "general_hint"},
		{"language_label", "language"},
		{"general_background_title", "general_background_title"},
		{"general_background_hint", "general_background_hint"},
		{"status_title", "status"},
	} {
		controlKey, textKey := item[0], item[1]
		if hwnd := state.localized[controlKey]; hwnd != 0 {
			state.setText(hwnd, state.tr(textKey))
		}
	}
	if state.selectedProfileID == "" {
		state.setText(state.saveProfileBtn, state.tr("add_profile"))
	} else {
		state.setText(state.saveProfileBtn, state.tr("update_profile"))
	}

	policy := state.currentConflictPolicy()
	procSettingsSendMessage.Call(uintptr(state.conflict), settingsCBReset, 0, 0)
	state.comboAdd(state.conflict, state.tr("conflict_stop"))
	state.comboAdd(state.conflict, state.tr("conflict_skip"))
	state.comboAdd(state.conflict, state.tr("conflict_replace"))
	state.setConflictPolicy(policy)

	if state.language == uiLanguageRussian {
		procSettingsSendMessage.Call(uintptr(state.languageCombo), settingsCBSetCurSel, 0, 0)
	} else {
		procSettingsSendMessage.Call(uintptr(state.languageCombo), settingsCBSetCurSel, 1, 0)
	}
}

func (state *windowsSettingsUI) changeLanguage() {
	index := state.comboSelection(state.languageCombo)
	language := uiLanguageRussian
	if index == 1 {
		language = uiLanguageEnglish
	}
	if language == state.language {
		return
	}
	settings, err := windowsclient.LoadClientSettings(state.settingsPath)
	if err != nil {
		state.showError(fmt.Sprintf("%s: %v", state.tr("error_prefix"), err))
		return
	}
	settings.Language = language
	if err := windowsclient.SaveClientSettings(state.settingsPath, settings); err != nil {
		state.showError(fmt.Sprintf("%s: %v", state.tr("error_prefix"), err))
		return
	}
	state.language = language
	state.applyLanguageTexts()
	_ = state.reloadProfiles(state.selectedProfileID)
	state.refreshAgentStatus()
	state.refreshBackupProfiles(state.selectedProfileID)
	state.refreshOverview()
	state.setStatus(state.tr("ready"))
	notifyTrayLanguageChanged()
}

func notifyTrayLanguageChanged() {
	className, err := windows.UTF16PtrFromString("HomeAIWindowsSyncTray")
	if err != nil {
		return
	}
	hwnd, _, _ := procSettingsFindWindow.Call(uintptr(unsafe.Pointer(className)), 0)
	if hwnd != 0 {
		procSettingsPostMessage.Call(hwnd, trayStatusMessage, 0, 0)
	}
}
