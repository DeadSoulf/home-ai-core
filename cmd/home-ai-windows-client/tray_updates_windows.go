//go:build windows

package main

import (
	"context"
	"time"
	"unsafe"

	"github.com/DeadSoulf/home-ai-core/internal/version"
	"github.com/DeadSoulf/home-ai-core/internal/windowsclient"
	"golang.org/x/sys/windows"
)

const settingsMBSetForeground = 0x00010000

func trayConfiguredReleaseChannel() string {
	path, err := defaultClientSettingsPath()
	if err != nil {
		return windowsclient.ReleaseChannelBeta
	}
	settings, err := windowsclient.LoadClientSettings(path)
	if err != nil {
		return windowsclient.ReleaseChannelBeta
	}
	channel, err := windowsclient.NormalizeReleaseChannel(settings.UpdateChannel)
	if err != nil {
		return windowsclient.ReleaseChannelBeta
	}
	return channel
}

func (state *windowsAgentTray) postTrayUpdateResult(title, message string, failed bool) {
	state.statusMu.Lock()
	state.updateCheckBusy = false
	state.notifyTitle = title
	state.notifyText = message
	state.notifyFlags = niifInfo
	if failed {
		state.notifyFlags = niifError
	}
	state.statusMu.Unlock()
	if state.hwnd != 0 {
		procPostMessageW.Call(uintptr(state.hwnd), trayStatusMessage, 0, 0)
	}
}

func (state *windowsAgentTray) checkTrayUpdates() {
	state.statusMu.Lock()
	if state.updateCheckBusy {
		state.statusMu.Unlock()
		return
	}
	state.updateCheckBusy = true
	state.statusMu.Unlock()
	channel := trayConfiguredReleaseChannel()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		release, err := windowsclient.DiscoverWindowsClientRelease(ctx, nil, channel)
		select {
		case <-state.done:
			return
		default:
		}
		language := preferredUILanguage()
		if err != nil {
			state.postTrayUpdateResult(
				textForLanguage(language, "tray_update_error"), safeActivityText(err.Error(), 150), true)
			return
		}
		body := textForLanguagef(language, "tray_update_current", channel)
		if windowsclient.CompareWindowsClientVersions(release.Version, version.Version) > 0 {
			body = textForLanguagef(language, "tray_update_available", release.Version, channel)
		}
		state.postTrayUpdateResult(textForLanguage(language, "tray_update_checking"), body, false)
	}()
}

func (state *windowsAgentTray) installTrayUpdate() {
	language := preferredUILanguage()
	message, _ := windows.UTF16PtrFromString(textForLanguage(language, "update_unsigned_confirmation"))
	title, _ := windows.UTF16PtrFromString(textForLanguage(language, "updates_title"))
	yes, _, _ := procSettingsMessageBox.Call(uintptr(state.hwnd), uintptr(unsafe.Pointer(message)),
		uintptr(unsafe.Pointer(title)), settingsMBYesNo|settingsMBIconWarning|settingsMBSetForeground)
	if yes != settingsIDYes {
		return
	}
	if err := launchWindowsClientUpdate(trayConfiguredReleaseChannel()); err != nil {
		state.postTrayUpdateResult(textForLanguage(language, "tray_update_error"),
			safeActivityText(err.Error(), 150), true)
	} else {
		state.postTrayUpdateResult(textForLanguage(language, "update_started"),
			textForLanguage(language, "update_started"), false)
	}
}
