package main

import (
	"slices"
	"strings"
	"testing"
)

func TestRenderAndParseNetworkdStaticProfile(t *testing.T) {
	config := renderNetworkdProfile(
		"enp3s0",
		"static",
		"192.168.50.10/24",
		"192.168.50.1",
		[]string{"1.1.1.1", "8.8.8.8"},
	)
	if !strings.Contains(config, "Name=enp3s0") {
		t.Fatal("interface match missing")
	}
	method, address, gateway, dns := parseNetworkdProfile(config)
	if method != "static" {
		t.Fatalf("method = %q, want static", method)
	}
	if address != "192.168.50.10/24" {
		t.Fatalf("address = %q", address)
	}
	if gateway != "192.168.50.1" {
		t.Fatalf("gateway = %q", gateway)
	}
	if !slices.Equal(dns, []string{"1.1.1.1", "8.8.8.8"}) {
		t.Fatalf("dns = %#v", dns)
	}
}

func TestRenderAndParseNetworkdDHCPProfile(t *testing.T) {
	config := renderNetworkdProfile("eth0", "dhcp", "", "", []string{"9.9.9.9"})
	method, address, gateway, dns := parseNetworkdProfile(config)
	if method != "dhcp" {
		t.Fatalf("method = %q, want dhcp", method)
	}
	if address != "" || gateway != "" {
		t.Fatalf("unexpected static values: address=%q gateway=%q", address, gateway)
	}
	if !slices.Equal(dns, []string{"9.9.9.9"}) {
		t.Fatalf("dns = %#v", dns)
	}
}

func TestValidateDNSAddresses(t *testing.T) {
	got, err := validateDNSAddresses([]string{" 1.1.1.1 ", "8.8.8.8", "1.1.1.1", ""})
	if err != nil {
		t.Fatalf("validateDNSAddresses() error = %v", err)
	}
	if !slices.Equal(got, []string{"1.1.1.1", "8.8.8.8"}) {
		t.Fatalf("dns = %#v", got)
	}
	if _, err := validateDNSAddresses([]string{"resolver.example"}); err == nil {
		t.Fatal("hostname DNS value was accepted")
	}
}

func TestTransientHostCommand(t *testing.T) {
	got := transientHostCommand("/usr/sbin/ip", "link", "show", "eth0")
	want := []string{
		"--quiet",
		"--wait",
		"--pipe",
		"--collect",
		"--service-type=exec",
		"--",
		"/usr/sbin/ip",
		"link",
		"show",
		"eth0",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("transientHostCommand() = %#v, want %#v", got, want)
	}
}
