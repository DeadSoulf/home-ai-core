package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAIEndpointRequiresManagedContainerAndMatchingNetwork(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX Docker stub")
	}
	dir := t.TempDir()
	dockerPath := filepath.Join(dir, "docker")
	write := func(id, ip, networkID string, running bool) {
		t.Helper()
		meta := map[string]any{
			"State": map[string]any{"Running": running},
			"Config": map[string]any{"Labels": map[string]string{
				"home-ai.managed": "true", "home-ai.module.id": id,
			}},
			"NetworkSettings": map[string]any{"Networks": map[string]any{
				"home-ai-modules": map[string]string{"IPAddress": ip, "NetworkID": networkID},
			}},
		}
		raw, err := json.Marshal(meta)
		if err != nil {
			t.Fatal(err)
		}
		script := "#!/bin/sh\ncase \"$1\" in\n inspect) printf '%s\\n' '" + string(raw) + "';;\n network) printf 'true|network-id\\n';;\n *) exit 2;;\nesac\n"
		if err := os.WriteFile(dockerPath, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	write("ai.agent", "172.19.0.6", "network-id", true)
	address, err := inspectManagedModuleAddress(context.Background(), "ai.agent")
	if err != nil || address != "172.19.0.6" {
		t.Fatalf("valid agent: %q %v", address, err)
	}
	if address, err := inspectManagedModuleAddress(context.Background(), "attacker.module"); err == nil || address != "" {
		t.Fatal("unapproved module discovery accepted")
	}
	for _, tc := range []struct {
		id, ip, network string
		running         bool
	}{
		{"nvr.module", "172.19.0.6", "network-id", true},
		{"ai.agent", "127.0.0.1", "network-id", true},
		{"ai.agent", "8.8.8.8", "network-id", true},
		{"ai.agent", "172.19.0.6", "untrusted-id", true},
		{"ai.agent", "172.19.0.6", "network-id", false},
	} {
		write(tc.id, tc.ip, tc.network, tc.running)
		if address, err := inspectManagedModuleAddress(context.Background(), "ai.agent"); err == nil || strings.TrimSpace(address) != "" {
			t.Fatalf("untrusted AI endpoint accepted: %s (%+v)", address, tc)
		}
	}
}
