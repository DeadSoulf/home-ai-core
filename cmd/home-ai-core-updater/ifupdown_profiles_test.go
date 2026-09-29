package main

import (
	"slices"
	"strings"
	"testing"
)

func TestParseIfupdownStaticProfile(t *testing.T) {
	config := `# manual config
auto eno1
iface eno1 inet static
    address 192.168.10.20
    netmask 255.255.255.0
    gateway 192.168.10.1
    dns-nameservers 1.1.1.1 8.8.8.8

iface eno1 inet6 auto
`
	stanzas := parseIfupdownFile("/etc/network/interfaces", config, false)
	if len(stanzas) != 2 {
		t.Fatalf("stanzas = %d, want 2: %#v", len(stanzas), stanzas)
	}
	got := stanzas[0]
	if got.Interface != "eno1" || got.Family != "inet" || got.Method != "static" {
		t.Fatalf("unexpected stanza: %#v", got)
	}
	if normalizeIfupdownAddress(got.Address, got.Netmask) != "192.168.10.20/24" {
		t.Fatalf("normalized address = %q", normalizeIfupdownAddress(got.Address, got.Netmask))
	}
	if got.Gateway != "192.168.10.1" {
		t.Fatalf("gateway = %q", got.Gateway)
	}
	if !slices.Equal(got.DNS, []string{"1.1.1.1", "8.8.8.8"}) {
		t.Fatalf("dns = %#v", got.DNS)
	}
	if got.Owned {
		t.Fatal("manual stanza reported as Home-AI owned")
	}
}

func TestParseIfupdownHomeAIProfile(t *testing.T) {
	config := renderIfupdownProfile(
		"eno2",
		"static",
		"10.10.20.5/24",
		"10.10.20.1",
		[]string{"9.9.9.9"},
	)
	stanzas := parseIfupdownFile(ifupdownOwnedPath("eno2"), config, true)
	if len(stanzas) != 1 {
		t.Fatalf("stanzas = %d, want 1", len(stanzas))
	}
	got := stanzas[0]
	if !got.Owned || got.Address != "10.10.20.5/24" || got.Gateway != "10.10.20.1" {
		t.Fatalf("unexpected Home-AI stanza: %#v", got)
	}
	if !strings.Contains(config, ifupdownManagedMarker) {
		t.Fatal("managed marker missing")
	}
}

func TestRenderIfupdownDHCPProfile(t *testing.T) {
	config := renderIfupdownProfile("eno3", "dhcp", "", "", []string{"1.1.1.1", "8.8.8.8"})
	for _, want := range []string{
		ifupdownManagedMarker,
		"auto eno3",
		"iface eno3 inet dhcp",
		"dns-nameservers 1.1.1.1 8.8.8.8",
	} {
		if !strings.Contains(config, want) {
			t.Fatalf("rendered DHCP profile missing %q:\n%s", want, config)
		}
	}
	if strings.Contains(config, "gateway ") || strings.Contains(config, "address ") {
		t.Fatalf("DHCP profile contains static configuration:\n%s", config)
	}
}

func TestEnsureIfupdownIncludeContentPreservesExistingConfiguration(t *testing.T) {
	original := `auto lo
iface lo inet loopback

allow-hotplug eno1
iface eno1 inet dhcp
`
	updated, changed := ensureIfupdownIncludeContent(original)
	if !changed {
		t.Fatal("include was not added")
	}
	if !strings.HasPrefix(updated, strings.TrimRight(original, "\n")) {
		t.Fatalf("existing configuration was changed:\n%s", updated)
	}
	if !strings.Contains(updated, ifupdownIncludeBegin) ||
		!strings.Contains(updated, ifupdownIncludeLine) ||
		!strings.Contains(updated, ifupdownIncludeEnd) {
		t.Fatalf("managed include block missing:\n%s", updated)
	}

	second, changed := ensureIfupdownIncludeContent(updated)
	if changed || second != updated {
		t.Fatal("include insertion is not idempotent")
	}
}

func TestIfupdownIncludesDir(t *testing.T) {
	cases := []string{
		"source /etc/network/interfaces.d/*\n",
		"source-directory /etc/network/interfaces.d\n",
	}
	for _, input := range cases {
		if !ifupdownIncludesDir(input) {
			t.Fatalf("include not detected in %q", input)
		}
	}
	if ifupdownIncludesDir("# source /etc/network/interfaces.d/*\n") {
		t.Fatal("commented include was treated as active")
	}
}

func TestIfupdownNetmaskPrefix(t *testing.T) {
	cases := map[string]int{
		"255.255.255.0":   24,
		"255.255.255.128": 25,
		"24":              24,
	}
	for input, want := range cases {
		got, ok := ifupdownNetmaskPrefix(input)
		if !ok || got != want {
			t.Fatalf("ifupdownNetmaskPrefix(%q) = %d, %v; want %d, true", input, got, ok, want)
		}
	}
	if _, ok := ifupdownNetmaskPrefix("255.0.255.0"); ok {
		t.Fatal("non-contiguous netmask accepted")
	}
}

func TestValidIfupdownIncludeName(t *testing.T) {
	valid := []string{"eno1", "50-home-ai-eno1", "interfaces-extra"}
	for _, name := range valid {
		if !validIfupdownIncludeName(name) {
			t.Fatalf("valid include name rejected: %q", name)
		}
	}
	invalid := []string{".hidden", "eno1~", "eno1.bak", "eno1.tmp", "eno1.dpkg-old"}
	for _, name := range invalid {
		if validIfupdownIncludeName(name) {
			t.Fatalf("backup/temp include name accepted: %q", name)
		}
	}
}
