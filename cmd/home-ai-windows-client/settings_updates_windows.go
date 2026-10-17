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

type settingsUpdateInstallResult struct {
	path    string
	version string
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
	if state.updateBusy {
		return
	}
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
	state.updateRelease = windowsclient.WindowsClientRelease{}
	state.updateLatestVersion = ""
	state.updateCheckedChannel = ""
	state.updateError = ""
	state.updateWasInstalled = false
	state.setControlEnabled(state.updateInstallBtn, false)
	state.renderUpdateStatus()
}

func (state *windowsSettingsUI) renderUpdateStatus() {
	if state.updateStatusLabel == 0 {
		return
	}
	label := state.trf("update_not_checked", version.Version)
	switch {
	case state.updateBusy && state.updateInstalling:
		label = state.tr("update_downloading")
	case state.updateBusy:
		label = state.tr("update_checking")
	case state.updateError != "":
		label = state.tr("update_install_failed") + ": " + safeActivityText(state.updateError, 120)
	case state.updateWasInstalled:
		label = state.trf("update_completed", version.Version)
	case state.updateLatestVersion != "" && state.updateCheckedChannel == state.selectedUpdateChannel():
		if state.updateAvailable {
			label = state.trf("update_available", version.Version, state.updateLatestVersion)
		} else {
			label = state.trf("update_current", version.Version, state.updateLatestVersion)
		}
	}
	state.setText(state.updateStatusLabel, label)
}

func (state *windowsSettingsUI) beginUpdateCheck() {
	if state.updateBusy {
		return
	}
	state.updateBusy = true
	state.updateInstalling = false
	state.updateError = ""
	state.updateAvailable = false
	state.updateWasInstalled = false
	state.updateRelease = windowsclient.WindowsClientRelease{}
	state.updateLatestVersion = ""
	state.updateCheckedChannel = ""
	state.setControlEnabled(state.updateChannelCombo, false)
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
		state.updateRelease = windowsclient.WindowsClientRelease{}
	} else {
		state.updateRelease = result.release
		state.updateLatestVersion = result.release.Version
		state.updateAvailable = windowsclient.CompareWindowsClientVersions(result.release.Version, version.Version) > 0
	}
	state.setControlEnabled(state.updateChannelCombo, true)
	state.setControlEnabled(state.updateCheckBtn, true)
	state.setControlEnabled(state.updateInstallBtn, state.updateAvailable && result.channel == state.selectedUpdateChannel())
	state.renderUpdateStatus()
}

// The GUI stays visible through discovery, download and SHA-256 verification.
// Only after a verified, separate installer has actually started do we close
// the old GUI so that Windows can replace its executable.
func (state *windowsSettingsUI) startUpdateFromUI() {
	if !state.updateAvailable || state.updateBusy ||
		state.updateCheckedChannel != state.selectedUpdateChannel() ||
		state.updateRelease.Version == "" {
		return
	}
	if !state.confirm(state.tr("update_unsigned_confirmation")) {
		return
	}
	state.updateBusy = true
	state.updateInstalling = true
	state.updateError = ""
	state.setControlEnabled(state.updateChannelCombo, false)
	state.setControlEnabled(state.updateCheckBtn, false)
	state.setControlEnabled(state.updateInstallBtn, false)
	state.renderUpdateStatus()

	release := state.updateRelease
	hwnd := uintptr(state.hwnd)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		path, _, err := windowsclient.DownloadWindowsClientRelease(ctx, nil, release)
		state.asyncMu.Lock()
		state.updateInstallResult = &settingsUpdateInstallResult{
			path: path, version: release.Version, err: err,
		}
		state.asyncMu.Unlock()
		procSettingsPostMessage.Call(hwnd, settingsWMUpdateInstallDone, 0, 0)
	}()
}

func (state *windowsSettingsUI) finishUpdateInstallation() {
	state.asyncMu.Lock()
	result := state.updateInstallResult
	state.updateInstallResult = nil
	state.asyncMu.Unlock()
	if result == nil {
		return
	}
	state.updateBusy = false
	state.updateInstalling = false
	state.setControlEnabled(state.updateChannelCombo, true)
	state.setControlEnabled(state.updateCheckBtn, true)
	state.setControlEnabled(state.updateInstallBtn,
		state.updateAvailable && state.updateCheckedChannel == state.selectedUpdateChannel())
	if result.err != nil {
		state.updateError = result.err.Error()
		state.renderUpdateStatus()
		state.showTechnicalError(result.err)
		return
	}
	if err := launchVerifiedWindowsClientInstall(result.path, result.version); err != nil {
		state.updateError = err.Error()
		state.renderUpdateStatus()
		state.showTechnicalError(err)
		return
	}
	// The new installer will reopen the stable installed GUI when it succeeds,
	// or show a foreground diagnostic if installing fails.
	state.setStatus(state.tr("update_installer_started"))
	procSettingsDestroyWindow.Call(uintptr(state.hwnd))
}
