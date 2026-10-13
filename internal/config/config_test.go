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

func TestSecureDefaultPortAndPlaintextRestriction(t *testing.T) {
	t.Setenv("HOME_AI_LISTEN", "")
	t.Setenv("HOME_AI_TLS_CERT", "")
	t.Setenv("HOME_AI_TLS_KEY", "")
	cfg, err := Load(nil)
	if err != nil || cfg.ListenAddress != "0.0.0.0:82" || cfg.InsecureLoopback {
		t.Fatalf("insecure defaults: %#v %v", cfg, err)
	}
	for _, address := range []string{"0.0.0.0:82", "[::]:82", "192.168.1.1:82", "localhost:82"} {
		if _, err := Load([]string{"--listen", address, "--allow-insecure-loopback"}); err == nil {
			t.Fatalf("plaintext allowed for %s", address)
		}
	}
	if _, err := Load([]string{"--listen", "127.0.0.1:18082", "--allow-insecure-loopback"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Load([]string{"--tls-cert", "only-one.pem"}); err == nil {
		t.Fatal("accepted an incomplete explicit TLS configuration")
	}
}
