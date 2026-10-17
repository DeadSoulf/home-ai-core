//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	settingsTransferTimerID = 8
	settingsPBMSetRange32   = 0x0406
	settingsPBMSetPos       = 0x0402
	settingsLBGetTopIndex   = 0x018E
	settingsLBSetTopIndex   = 0x0197
)

type settingsInitCommonControlsEx struct {
	size    uint32
	classes uint32
}

var procSettingsInitCommonControls = windows.NewLazySystemDLL("comctl32.dll").NewProc("InitCommonControlsEx")

func settingsInitCommonControls() error {
	classes := settingsInitCommonControlsEx{size: uint32(unsafe.Sizeof(settingsInitCommonControlsEx{})), classes: 0x0020 | 0x4000}
	ok, _, err := procSettingsInitCommonControls.Call(uintptr(unsafe.Pointer(&classes)))
	if ok == 0 {
		return fmt.Errorf("initialize Windows Common Controls: %w", err)
	}
	return nil
}

func transferStatusText(state *windowsSettingsUI, raw string) string {
	switch raw {
	case "running", "completed", "failed", "copied", "skipped", "unchanged":
		return state.tr("transfer_" + raw)
	default:
		return raw
	}
}

func transferPercent(item transferActivity) int {
	switch {
	case item.Status == "completed":
		return 100
	case item.TotalBytes > 0:
		p := item.UploadedBytes * 100 / item.TotalBytes
		if p < 0 {
			return 0
		}
		if p > 100 {
			return 100
		}
		return int(p)
	case item.ItemsTotal > 0:
		p := item.ItemsDone * 100 / item.ItemsTotal
		if p > 100 {
			p = 100
		}
		return p
	default:
		return 0
	}
}

func (state *windowsSettingsUI) refreshTransfers() {
	if state.hwnd == 0 || state.page != settingsPageTransfers || state.transferProgress == 0 {
		return
	}
	item, err := readTransferProgress(state.configPath)
	percent := 0
	message := state.tr("transfer_idle")
	if err == nil && !item.At.IsZero() {
		percent = transferPercent(item)
		status := transferStatusText(state, item.Status)
		if item.Status == "running" && time.Since(item.At) > 2*time.Minute {
			status = state.tr("transfer_stale")
			percent = 0
		}
		details := []string{status}
		if item.ItemsTotal > 0 {
			details = append(details, fmt.Sprintf("%d/%d", item.ItemsDone, item.ItemsTotal))
		}
		if item.Path != "" {
			details = append(details, item.Path)
		}
		if item.TotalBytes > 0 {
			details = append(details, state.trf("transfer_bytes", dashboardBytes(item.UploadedBytes), dashboardBytes(item.TotalBytes)))
		}
		if item.Status == "running" && time.Since(item.At) <= 2*time.Minute && item.SpeedBytesPerSec > 0 {
			details = append(details, state.trf("transfer_speed", dashboardBytes(item.SpeedBytesPerSec)))
			if item.ETASeconds > 0 {
				details = append(details, state.trf("transfer_remaining", time.Duration(item.ETASeconds)*time.Second))
			}
		}
		if item.Error != "" {
			details = append(details, item.Error)
		}
		message = strings.Join(details, "  ·  ")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		message = state.tr("transfer_unavailable")
	}
	procSettingsSendMessage.Call(uintptr(state.transferProgress), settingsPBMSetRange32, 0, 100)
	procSettingsSendMessage.Call(uintptr(state.transferProgress), settingsPBMSetPos, uintptr(percent), 0)
	state.setText(state.transferDetails, message)

	if state.transferHistory == 0 {
		return
	}
	events, historyErr := readTransferHistory(state.configPath, 50)
	if historyErr != nil {
		state.setStatus(state.tr("transfer_unavailable") + ": " + historyErr.Error())
		return
	}
	// Preserve the scroll position while the timer refreshes progress.
	top, _, _ := procSettingsSendMessage.Call(uintptr(state.transferHistory), settingsLBGetTopIndex, 0, 0)
	procSettingsSendMessage.Call(uintptr(state.transferHistory), settingsLBReset, 0, 0)
	if len(events) == 0 {
		state.listAdd(state.transferHistory, state.tr("transfer_history_empty"))
	} else {
		for _, event := range events {
			label := fmt.Sprintf("%s  ·  %s  ·  %s",
				event.At.Local().Format("02.01 15:04:05"),
				transferStatusText(state, event.Status),
				event.Path)
			if event.Error != "" {
				label += " — " + event.Error
			}
			state.listAdd(state.transferHistory, label)
		}
	}
	procSettingsSendMessage.Call(uintptr(state.transferHistory), settingsLBSetTopIndex, top, 0)
}
