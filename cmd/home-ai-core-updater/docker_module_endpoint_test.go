package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInspectNVRModuleAddressChecksManagedIdentityAndNetwork(t *testing.T) {
	if runtime.GOOS == "windows" { t.Skip("POSIX Docker stub") }
	dir := t.TempDir()
	dockerPath := filepath.Join(dir, "docker")
	write := func(managed, networkID, address, running string) {
		t.Helper()
		body := `#!/bin/sh
case "$1" in
 inspect)
  printf '%s\n' '{"State":{"Running":` + running + `},"Config":{"Labels":{"home-ai.managed":"` + managed + `","home-ai.module.id":"nvr.module"}},"NetworkSettings":{"Networks":{"home-ai-modules":{"IPAddress":"` + address + `","NetworkID":"` + networkID + `"}}}}';;
 network)
  printf '%s\n' 'true|network-id';;
 *) exit 2;;
esac
`
		if err := os.WriteFile(dockerPath, []byte(body), 0o755); err != nil { t.Fatal(err) }
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	write("true", "network-id", "172.20.0.4", "true")
	address, err := inspectNVRModuleAddress(context.Background())
	if err != nil || address != "172.20.0.4" { t.Fatalf("valid endpoint: %q %v", address, err) }
	for _, tc := range []struct{ managed, networkID, address, running string }{
		{"false", "network-id", "172.20.0.4", "true"},
		{"true", "untrusted-id", "172.20.0.4", "true"},
		{"true", "network-id", "127.0.0.1", "true"},
		{"true", "network-id", "8.8.8.8", "true"},
		{"true", "network-id", "172.20.0.4", "false"},
	} {
		write(tc.managed, tc.networkID, tc.address, tc.running)
		if address, err := inspectNVRModuleAddress(context.Background()); err == nil {
			t.Fatalf("unsafe Docker endpoint accepted: %q (%+v)", address, tc)
		}
	}
	if address, err := inspectNVRModuleAddress(context.Background()); err == nil || strings.TrimSpace(address) != "" {
		t.Fatal("unexpected endpoint accepted")
	}
}
