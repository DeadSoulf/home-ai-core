//go:build windows

package main

import "testing"

func TestSameTraySyncConfig(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{a: `C:\Users\TexNik\AppData\Roaming\HomeAI\sync-profiles.json`,
			b: `c:\users\texnik\appdata\roaming\homeai\sync-profiles.json`, want: true},
		{a: `C:\HomeAI\tmp\..\sync.json`, b: `C:\HomeAI\sync.json`, want: true},
		{a: `C:\HomeAI\first.json`, b: `C:\HomeAI\second.json`, want: false},
		{a: "", b: `C:\HomeAI\sync.json`, want: false},
		{a: `C:\HomeAI\sync.json`, b: "", want: false},
	} {
		if got := sameTraySyncConfig(tc.a, tc.b); got != tc.want {
			t.Errorf("sameTraySyncConfig(%q,%q)=%v want %v", tc.a, tc.b, got, tc.want)
		}
	}
}
