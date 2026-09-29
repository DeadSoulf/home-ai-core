package systeminfo

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestNetworkInterfacesFromSysfsWithoutNetlink(t *testing.T) {
	root := t.TempDir()
	for _, tc := range []struct {
		name      string
		index     string
		mac       string
		mtu       string
		flags     string
		state     string
		speed     string
		ipv4      string
		wantUp    bool
		wantLoop  bool
		wantMulti bool
	}{
		{
			name:      "eno1",
			index:     "2",
			mac:       "00:11:22:33:44:55",
			mtu:       "1500",
			flags:     "0x1003",
			state:     "up",
			speed:     "1000",
			ipv4:      "192.168.10.23/24",
			wantUp:    true,
			wantMulti: true,
		},
		{
			name:     "lo",
			index:    "1",
			mac:      "00:00:00:00:00:00",
			mtu:      "65536",
			flags:    "0x9",
			state:    "unknown",
			speed:    "",
			ipv4:     "127.0.0.1/8",
			wantUp:   true,
			wantLoop: true,
		},
	} {
		dir := filepath.Join(root, tc.name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, filepath.Join(dir, "ifindex"), tc.index+"\n")
		writeTestFile(t, filepath.Join(dir, "address"), tc.mac+"\n")
		writeTestFile(t, filepath.Join(dir, "mtu"), tc.mtu+"\n")
		writeTestFile(t, filepath.Join(dir, "flags"), tc.flags+"\n")
		writeTestFile(t, filepath.Join(dir, "operstate"), tc.state+"\n")
		if tc.speed != "" {
			writeTestFile(t, filepath.Join(dir, "speed"), tc.speed+"\n")
		}
	}

	procInet6 := filepath.Join(t.TempDir(), "if_inet6")
	writeTestFile(
		t,
		procInet6,
		"fe80000000000000021122fffe334455 02 40 20 80 eno1\n"+
			"00000000000000000000000000000001 01 80 10 80 lo\n",
	)

	ipv4 := map[string]string{
		"eno1": "192.168.10.23/24",
		"lo":   "127.0.0.1/8",
	}
	got := networkInterfacesFromPaths(root, procInet6, func(name string) string {
		return ipv4[name]
	})

	if len(got) != 2 {
		t.Fatalf("interface count = %d, want 2: %#v", len(got), got)
	}
	if got[0].Name != "eno1" {
		t.Fatalf("first interface = %q, want eno1", got[0].Name)
	}
	if got[0].Index != 2 || got[0].MAC != "00:11:22:33:44:55" || got[0].MTU != 1500 {
		t.Fatalf("unexpected eno1 identity: %#v", got[0])
	}
	if !got[0].Up || got[0].Loopback || !got[0].Multicast {
		t.Fatalf("unexpected eno1 flags: %#v", got[0])
	}
	if got[0].SpeedBPS != 1_000_000_000 {
		t.Fatalf("eno1 speed = %d", got[0].SpeedBPS)
	}
	wantAddresses := []string{"192.168.10.23/24", "fe80::211:22ff:fe33:4455/64"}
	if !slices.Equal(got[0].Addresses, wantAddresses) {
		t.Fatalf("eno1 addresses = %#v, want %#v", got[0].Addresses, wantAddresses)
	}

	if got[1].Name != "lo" || !got[1].Loopback || !got[1].Up {
		t.Fatalf("unexpected loopback interface: %#v", got[1])
	}
}

func TestIPv6AddressesIgnoresMalformedRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "if_inet6")
	writeTestFile(
		t,
		path,
		"fe80000000000000021122fffe334455 02 40 20 80 eno1\n"+
			"bad row\n",
	)
	got := ipv6Addresses(path)
	want := []string{"fe80::211:22ff:fe33:4455/64"}
	if !slices.Equal(got["eno1"], want) {
		t.Fatalf("eno1 IPv6 = %#v, want %#v", got["eno1"], want)
	}
}
