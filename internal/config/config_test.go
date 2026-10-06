package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("HOME_AI_LISTEN", "")
	t.Setenv("HOME_AI_STATE_DIR", "")
	t.Setenv("HOME_AI_WEB_DIR", "")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.ListenAddress != defaultListen {
		t.Fatalf("ListenAddress = %q, want %q", cfg.ListenAddress, defaultListen)
	}
	if cfg.StateDir != defaultStateDir {
		t.Fatalf("StateDir = %q, want %q", cfg.StateDir, defaultStateDir)
	}
	if cfg.WebDir != defaultWebDir {
		t.Fatalf("WebDir = %q, want %q", cfg.WebDir, defaultWebDir)
	}
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv("HOME_AI_LISTEN", "127.0.0.1:9000")
	t.Setenv("HOME_AI_STATE_DIR", "/tmp/from-env")
	t.Setenv("HOME_AI_WEB_DIR", "/tmp/web-env")

	cfg, err := Load([]string{
		"-listen", "127.0.0.1:9100",
		"-state-dir", "/tmp/from-flag",
		"-web-dir", "/tmp/web-flag",
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.ListenAddress != "127.0.0.1:9100" {
		t.Fatalf("ListenAddress = %q", cfg.ListenAddress)
	}
	if cfg.StateDir != "/tmp/from-flag" {
		t.Fatalf("StateDir = %q", cfg.StateDir)
	}
	if cfg.WebDir != "/tmp/web-flag" {
		t.Fatalf("WebDir = %q", cfg.WebDir)
	}
}

