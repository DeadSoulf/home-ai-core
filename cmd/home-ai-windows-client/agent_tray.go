package main

import (
	"fmt"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/windowsclient"
)

type agentTrayRuntime struct {
	RunNow      <-chan struct{}
	Exit        <-chan struct{}
	ReportCycle func(manual bool, count int, err error)
	close       func() error
}

func (tray *agentTrayRuntime) Close() error {
	if tray == nil || tray.close == nil {
		return nil
	}
	return tray.close()
}

func agentTraySummary(configPath string, now time.Time) string {
	profiles, err := windowsclient.LoadSyncProfiles(configPath)
	if err != nil {
		return "profiles unavailable"
	}
	enabled := 0
	var latest *windowsclient.SyncProfile
	for i := range profiles {
		profile := &profiles[i]
		if !profile.Enabled {
			continue
		}
		enabled++
		if profile.LastAttemptAt == nil {
			continue
		}
		if latest == nil || latest.LastAttemptAt == nil || profile.LastAttemptAt.After(*latest.LastAttemptAt) {
			latest = profile
		}
	}
	if enabled == 0 {
		return "no enabled profiles"
	}
	if latest == nil || latest.LastAttemptAt == nil {
		return fmt.Sprintf("%d enabled · not synced yet", enabled)
	}
	result := "OK"
	if latest.LastError != "" {
		result = "FAILED"
	}
	when := latest.LastAttemptAt.In(now.Location()).Format("15:04")
	return fmt.Sprintf("%d enabled · last %s %s", enabled, result, when)
}
