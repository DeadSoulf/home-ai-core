package main

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestValidInterfaceName(t *testing.T) {
	valid := []string{"eth0", "enp3s0", "wg0", "br-home", "vlan.20"}
	for _, name := range valid {
		if !validInterfaceName(name) {
			t.Fatalf("validInterfaceName(%q) = false", name)
		}
	}
	invalid := []string{"", "interface-name-is-too-long", "bad/name", "bad name", "bad:name"}
	for _, name := range invalid {
		if validInterfaceName(name) {
			t.Fatalf("validInterfaceName(%q) = true", name)
		}
	}
}

func TestWireGuardKeyValidation(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	if err := validateWireGuardKey(key); err != nil {
		t.Fatalf("valid key rejected: %v", err)
	}
	if err := validateWireGuardKey("not-a-key"); err == nil {
		t.Fatal("invalid key accepted")
	}
}

func TestWireGuardPeerConfigRoundTrip(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	config := "[Interface]\nAddress = 10.7.0.1/24\nListenPort = 51820\nPrivateKey = secret\n"

	updated, err := appendWireGuardPeer(
		config,
		key,
		"",
		[]string{"10.7.0.2/32"},
		"vpn.example.test:51820",
		25,
	)
	if err != nil {
		t.Fatalf("appendWireGuardPeer() error = %v", err)
	}
	if !wireGuardConfigHasPeer(updated, key) {
		t.Fatal("peer missing after append")
	}
	if !strings.Contains(updated, "PersistentKeepalive = 25") {
		t.Fatal("keepalive missing from peer config")
	}
	if _, err := appendWireGuardPeer(updated, key, "", []string{"10.7.0.2/32"}, "", 0); err == nil {
		t.Fatal("duplicate peer was accepted")
	}

	removed, found := removeWireGuardPeer(updated, key)
	if !found {
		t.Fatal("peer was not found for removal")
	}
	if wireGuardConfigHasPeer(removed, key) {
		t.Fatal("peer still present after removal")
	}
	if !strings.Contains(removed, "[Interface]") {
		t.Fatal("interface section was damaged by peer removal")
	}
}

func TestWireGuardInterfaceConfig(t *testing.T) {
	address, port := wireGuardInterfaceConfig(
		"[Interface]\nAddress = 10.9.0.1/24\nListenPort = 51900\nPrivateKey = secret\n",
	)
	if address != "10.9.0.1/24" {
		t.Fatalf("address = %q", address)
	}
	if port != 51900 {
		t.Fatalf("port = %d", port)
	}
}

func TestTransientPackageCommand(t *testing.T) {
	got := transientPackageCommand("/usr/bin/apt-get", "install", "-y", "wireguard-tools")
	want := []string{
		"--quiet",
		"--wait",
		"--pipe",
		"--collect",
		"--service-type=exec",
		"--setenv=DEBIAN_FRONTEND=noninteractive",
		"--",
		"/usr/bin/apt-get",
		"install",
		"-y",
		"wireguard-tools",
	}
	if len(got) != len(want) {
		t.Fatalf("command length = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("command[%d] = %q, want %q; full command %#v", i, got[i], want[i], got)
		}
	}
}
