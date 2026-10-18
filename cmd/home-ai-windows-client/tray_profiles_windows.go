//go:build windows

package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"github.com/DeadSoulf/home-ai-core/internal/windowsclient"
	"golang.org/x/sys/windows"
)

const (
	trayProfileToggleBase = 2100
	trayProfileMenuLimit  = 24
	mfPopup               = 0x00000010
)

func appendTraySubmenu(parent, submenu uintptr, label string) {
	name, err := windows.UTF16PtrFromString(label)
	if err != nil {
		return
	}
	procAppendMenuW.Call(parent, mfPopup|mfString, submenu, uintptr(unsafe.Pointer(name)))
}

func trayProfileName(profile windowsclient.SyncProfile) string {
	name := filepath.Base(profile.Source)
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = profile.Source
	}
	runes := []rune(strings.ReplaceAll(name, "&", "&&"))
	if len(runes) > 36 {
		name = string(runes[:35]) + "…"
	} else {
		name = string(runes)
	}
	return name
}

func trayProfileStatus(profile windowsclient.SyncProfile, language string) (summary, last, next string) {
	summary = textForLanguage(language, "profile_off")
	if profile.Enabled {
		summary = textForLanguage(language, "profile_on")
	}
	last = textForLanguage(language, "tray_profile_never")
	if profile.LastAttemptAt != nil {
		result := textForLanguage(language, "recent_activity_ok")
		if profile.LastError != "" {
			result = textForLanguage(language, "recent_activity_error")
		}
		last = textForLanguagef(language, "tray_profile_last", result,
			profile.LastAttemptAt.Local().Format("02.01 15:04"))
	}
	next = textForLanguage(language, "tray_profile_paused")
	if profile.Enabled {
		if due := profile.NextDue(); !due.IsZero() {
			next = textForLanguagef(language, "tray_profile_next",
				due.Local().Format("02.01 15:04"))
		}
	}
	return
}

func (state *windowsAgentTray) appendProfileMenu(parent uintptr, language string) {
	state.trayProfileIDs = state.trayProfileIDs[:0]
	profiles, loadErr := windowsclient.LoadSyncProfiles(state.configPath)
	submenu, _, _ := procCreatePopupMenu.Call()
	if submenu == 0 {
		return
	}
	if loadErr != nil {
		appendTrayMenu(submenu, mfString|mfGrayed, 0, textForLanguage(language, "tray_profile_unavailable"))
	} else if len(profiles) == 0 {
		appendTrayMenu(submenu, mfString|mfGrayed, 0, textForLanguage(language, "tray_profile_empty"))
	} else {
		for i, profile := range profiles {
			if i >= trayProfileMenuLimit {
				appendTrayMenu(submenu, mfString|mfGrayed, 0,
					textForLanguagef(language, "tray_profile_more", len(profiles)-i))
				break
			}
			entry, _, _ := procCreatePopupMenu.Call()
			if entry == 0 {
				break
			}
			status, last, next := trayProfileStatus(profile, language)
			appendTrayMenu(entry, mfString|mfGrayed, 0, last)
			appendTrayMenu(entry, mfString|mfGrayed, 0, next)
			if profile.LastError != "" {
				errorSummary := safeActivityText(profile.LastError, 72)
				appendTrayMenu(entry, mfString|mfGrayed, 0, errorSummary)
			}
			appendTrayMenu(entry, mfSeparator, 0, "")
			toggle := textForLanguage(language, "tray_profile_enable")
			if profile.Enabled {
				toggle = textForLanguage(language, "tray_profile_disable")
			}
			id := uint16(trayProfileToggleBase + len(state.trayProfileIDs))
			appendTrayMenu(entry, mfString, id, toggle)
			appendTraySubmenu(submenu, entry, fmt.Sprintf("%s — %s", trayProfileName(profile), status))
			state.trayProfileIDs = append(state.trayProfileIDs, profile.ID)
		}
	}
	appendTraySubmenu(parent, submenu, textForLanguage(language, "tray_profiles"))
}

func (state *windowsAgentTray) toggleTrayProfile(command uint16) bool {
	if command < trayProfileToggleBase || int(command-trayProfileToggleBase) >= len(state.trayProfileIDs) {
		return false
	}
	id := state.trayProfileIDs[int(command-trayProfileToggleBase)]
	language := preferredUILanguage()
	profiles, err := windowsclient.LoadSyncProfiles(state.configPath)
	if err == nil {
		found := false
		for _, profile := range profiles {
			if profile.ID == id {
				found = true
				err = windowsclient.SetSyncProfileEnabled(state.configPath, id, !profile.Enabled)
				break
			}
		}
		if !found {
			err = fmt.Errorf("profile no longer exists")
		}
	}
	state.statusMu.Lock()
	state.statusText = agentTraySummaryLocalized(state.configPath, time.Now(), language)
	if err != nil {
		state.notifyTitle = textForLanguage(language, "tray_profile_error")
		state.notifyText = safeActivityText(err.Error(), 160)
		state.notifyFlags = niifError
	} else {
		state.notifyTitle = ""
		state.notifyText = ""
		state.notifyFlags = 0
	}
	state.statusMu.Unlock()
	state.applyStatus()
	return true
}
