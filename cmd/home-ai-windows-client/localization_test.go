package main

import "testing"

func TestWindowsClientTranslations(t *testing.T) {
	if got := textForLanguage("ru", "settings"); got != "Настройки" {
		t.Fatalf("Russian settings label = %q", got)
	}
	if got := textForLanguage("en", "settings"); got != "Settings" {
		t.Fatalf("English settings label = %q", got)
	}
	if got := normalizeUILanguage("ru-RU"); got != uiLanguageRussian {
		t.Fatalf("ru-RU normalized to %q", got)
	}
	if got := normalizeUILanguage("de-DE"); got != uiLanguageEnglish {
		t.Fatalf("unsupported locale fallback = %q", got)
	}
}
