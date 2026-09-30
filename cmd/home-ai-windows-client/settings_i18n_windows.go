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
		{"language_label", "language"},
		{"server_label", "server"},
		{"connect_button", "connect_save"},
		{"user_label", "user"},
		{"password_label", "password"},
		{"password_hint", "password_hint"},
		{"remote_label", "remote_folder"},
		{"local_label", "local_source"},
		{"browse_button", "browse"},
		{"destination_label", "destination"},
		{"interval_label", "interval"},
		{"conflict_label", "conflict"},
		{"new_button", "new_clear"},
		{"enable_button", "enable"},
		{"disable_button", "disable"},
		{"delete_button", "delete"},
		{"refresh_button", "refresh"},
		{"profiles_label", "sync_profiles"},
		{"sync_button", "sync_now"},
		{"agent_enable", "enable_agent"},
		{"agent_disable", "disable_agent"},
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
