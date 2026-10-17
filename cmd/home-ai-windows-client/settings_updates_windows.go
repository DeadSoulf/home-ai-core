//go:build windows

package main

import (
	"context"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/version"
	"github.com/DeadSoulf/home-ai-core/internal/windowsclient"
)

type settingsUpdateCheckResult struct {
	release windowsclient.WindowsClientRelease
	channel string
	err     error
}

func (state *windowsSettingsUI) loadUpdateChannel() {
	settings, err := windowsclient.LoadClientSettings(state.settingsPath)
	if err != nil {
		return
	}
	channel, err := windowsclient.NormalizeReleaseChannel(settings.UpdateChannel)
	if err != nil {
		return
	}
	index := uintptr(1)
	if channel == windowsclient.ReleaseChannelStable {
		index = 0
	}
	procSettingsSendMessage.Call(uintptr(state.updateChannelCombo), settingsCBSetCurSel, index, 0)
	state.renderUpdateStatus()
}

func (state *windowsSettingsUI) selectedUpdateChannel() string {
	if state.comboSelection(state.updateChannelCombo) == 0 {
		return windowsclient.ReleaseChannelStable
	}
	return windowsclient.ReleaseChannelBeta
}

func (state *windowsSettingsUI) changeUpdateChannel() {
	settings, err := windowsclient.LoadClientSettings(state.settingsPath)
	if err != nil {
		state.showTechnicalError(err)
		return
	}
	settings.UpdateChannel = state.selectedUpdateChannel()
	if err := windowsclient.SaveClientSettings(state.settingsPath, settings); err != nil {
		state.showTechnicalError(err)
		return
	}
	state.updateAvailable = false
	state.updateLatestVersion = ""
	state.updateCheckedChannel = ""
	state.setControlEnabled(state.updateInstallBtn, false)
	state.renderUpdateStatus()
}

func (state *windowsSettingsUI) renderUpdateStatus() {
	if state.updateStatusLabel == 0 {
		return
	}
	label := state.trf("update_not_checked", version.Version)
	if state.updateBusy {
		label = state.tr("update_checking")
	} else if state.updateLatestVersion != "" && state.updateCheckedChannel == state.selectedUpdateChannel() {
		if state.updateAvailable {
			label = state.trf("update_available", version.Version, state.updateLatestVersion)
		} else {
			label = state.trf("update_current", version.Version, state.updateLatestVersion)
		}
	} else if state.updateError != "" {
		label = state.tr("update_check_failed") + ": " + safeActivityText(state.updateError, 120)
	}
	state.setText(state.updateStatusLabel, label)
}

func (state *windowsSettingsUI) beginUpdateCheck() {
	if state.updateBusy {
		return
	}
	state.updateBusy = true
	state.updateError = ""
	state.updateAvailable = false
	state.updateLatestVersion = ""
	state.updateCheckedChannel = ""
	state.setControlEnabled(state.updateCheckBtn, false)
	state.setControlEnabled(state.updateInstallBtn, false)
	state.renderUpdateStatus()
	channel := state.selectedUpdateChannel()
	hwnd := uintptr(state.hwnd)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		release, err := windowsclient.DiscoverWindowsClientRelease(ctx, nil, channel)
		state.asyncMu.Lock()
		state.updateResult = &settingsUpdateCheckResult{release: release, channel: channel, err: err}
		state.asyncMu.Unlock()
		procSettingsPostMessage.Call(hwnd, settingsWMUpdateCheckDone, 0, 0)
	}()
}

func (state *windowsSettingsUI) finishUpdateCheck() {
	state.asyncMu.Lock()
	result := state.updateResult
	state.updateResult = nil
	state.asyncMu.Unlock()
	if result == nil {
		return
	}
	state.updateBusy = false
	state.updateCheckedChannel = result.channel
	if result.err != nil {
		state.updateError = result.err.Error()
	} else {
		state.updateLatestVersion = result.release.Version
		state.updateAvailable = windowsclient.CompareWindowsClientVersions(result.release.Version, version.Version) > 0
	}
	state.setControlEnabled(state.updateCheckBtn, true)
	state.setControlEnabled(state.updateInstallBtn, state.updateAvailable && result.channel == state.selectedUpdateChannel())
	state.renderUpdateStatus()
}

func (state *windowsSettingsUI) startUpdateFromUI() {
	if !state.updateAvailable || state.updateBusy ||
		state.updateCheckedChannel != state.selectedUpdateChannel() {
		return
	}
	if !state.confirm(state.tr("update_unsigned_confirmation")) {
		return
	}
	if err := launchWindowsClientUpdate(state.selectedUpdateChannel()); err != nil {
		state.showTechnicalError(err)
		return
	}
	// Closing the GUI releases the executable so the existing bounded install
	// handoff can replace it. The independent CLI process writes update-log.
	procSettingsDestroyWindow.Call(uintptr(state.hwnd))
}
