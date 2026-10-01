//go:build windows

package main

import "testing"

func TestLoadHomeAIIcon(t *testing.T) {
	icon, err := loadHomeAIIcon(32)
	if err != nil {
		t.Fatalf("load HOME AI icon: %v", err)
	}
	if icon == 0 {
		t.Fatal("load HOME AI icon returned a zero handle")
	}
	destroyHomeAIIcon(icon)
}
