package main

import (
	"reflect"
	"testing"
)

func TestWindowsSettingsReopenArgs(t *testing.T) {
	cases := []struct {
		config, settings string
		want             []string
	}{
		{"", "", []string{"settings", "--updated"}},
		{"/tmp/sync.json", "", []string{"settings", "--updated", "--config", "/tmp/sync.json"}},
		{"C:\\Home AI\\sync.json", "C:\\Home AI\\ui.json", []string{"settings", "--updated", "--config", "C:\\Home AI\\sync.json", "--settings", "C:\\Home AI\\ui.json"}},
	}
	for _, tc := range cases {
		got := windowsSettingsReopenArgs(tc.config, tc.settings)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("reopen settings args = %v, want %v", got, tc.want)
		}
	}
}
