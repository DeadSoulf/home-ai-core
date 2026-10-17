package main

import (
	"reflect"
	"testing"
)

func TestWindowsSettingsReopenArgs(t *testing.T) {
	cases := []struct {
		config, settings string
		changed          bool
		want             []string
	}{
		{"", "", true, []string{"settings", "--updated"}},
		{"", "", false, []string{"settings", "--update-current"}},
		{"/tmp/sync.json", "", true, []string{"settings", "--updated", "--config", "/tmp/sync.json"}},
		{"C:\\Home AI\\sync.json", "C:\\Home AI\\ui.json", false, []string{"settings", "--update-current", "--config", "C:\\Home AI\\sync.json", "--settings", "C:\\Home AI\\ui.json"}},
	}
	for _, tc := range cases {
		got := windowsSettingsReopenArgs(tc.config, tc.settings, tc.changed)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("reopen settings args = %v, want %v", got, tc.want)
		}
	}
}
