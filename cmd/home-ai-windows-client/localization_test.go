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

func TestWindowsClientTranslationKeyParity(t *testing.T) {
	for key := range uiText[uiLanguageEnglish] {
		if _, ok := uiText[uiLanguageRussian][key]; !ok {
			t.Errorf("Russian translation missing for %q", key)
		}
	}
	for key := range uiText[uiLanguageRussian] {
		if _, ok := uiText[uiLanguageEnglish][key]; !ok {
			t.Errorf("English translation missing for %q", key)
		}
	}
}
