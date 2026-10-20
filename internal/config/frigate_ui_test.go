package config

import "testing"

func TestFrigateUIListenerIsIsolatedAndInheritsBindInterface(t *testing.T) {
	t.Setenv("HOME_AI_FRIGATE_UI_LISTEN", "")
	t.Setenv("HOME_AI_LISTEN", "0.0.0.0:82")
	cfg, err := Load(nil)
	if err != nil || cfg.FrigateUIListen != "0.0.0.0:8971" {
		t.Fatalf("wrong default: %#v %v", cfg, err)
	}
	cfg, err = Load([]string{"--listen", "127.0.0.1:18082", "--allow-insecure-loopback"})
	if err != nil || cfg.FrigateUIListen != "127.0.0.1:8971" {
		t.Fatal("development listener escaped loopback", err)
	}
	for _, listen := range []string{"0.0.0.0:82", "0.0.0.0:0", "0.0.0.0:99999", "127.0.0.1:8971", "attacker.test:8971"} {
		if _, err := Load([]string{"--frigate-ui-listen", listen}); err == nil {
			t.Fatal("invalid UI listener accepted", listen)
		}
	}
	if cfg, err := Load([]string{"--frigate-ui-listen", "off"}); err != nil || cfg.FrigateUIListen != "off" {
		t.Fatal("cannot disable optional gateway", err)
	}
	if cfg, err := Load([]string{"--frigate-ui-listen", "0.0.0.0:18971"}); err != nil || cfg.FrigateUIListen != "0.0.0.0:18971" {
		t.Fatal("cannot resolve a port conflict", err)
	}
}
