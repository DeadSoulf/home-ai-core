package nvrdiscovery

import (
	"bytes"
	"context"
	"encoding/binary"
	"net"
	"net/netip"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestSubnetForAddressIsBoundedAndPrivate(t *testing.T) {
	for _, tc := range []struct {
		address string
		mask    int
		want    string
	}{
		{"192.168.12.17", 16, "192.168.12.0/24"},
		{"10.3.4.5", 24, "10.3.4.0/24"},
		{"172.20.1.37", 28, "172.20.1.32/28"},
	} {
		got, ok := subnetForAddress(netip.MustParseAddr(tc.address), tc.mask)
		if !ok || got.String() != tc.want {
			t.Fatalf("%s/%d => %s (%v), want %s", tc.address, tc.mask, got, ok, tc.want)
		}
	}
	for _, ip := range []string{"8.8.8.8", "127.0.0.1", "169.254.4.2", "2001:db8::1"} {
		if _, ok := subnetForAddress(netip.MustParseAddr(ip), 24); ok {
			t.Fatalf("unsafe network %s accepted", ip)
		}
	}
	if _, ok := subnetForAddress(netip.MustParseAddr("192.168.1.8"), 16); !ok {
		t.Fatal("local network rejected")
	}
	if _, ok := subnetForAddress(netip.MustParseAddr("192.168.1.8"), 31); ok {
		t.Fatal("host-only interface mask should not be scanned")
	}
	for _, iface := range []string{"docker0", "veth01", "br-e22", "virbr0", "cni0"} {
		if !excludedInterface(iface) {
			t.Fatalf("Docker network exposed %s", iface)
		}
	}
}

func TestHostEnumerationExcludesBroadcastAndSelf(t *testing.T) {
	network := Network{ID: "eth0|192.168.4.0/29", CIDR: "192.168.4.0/29", Address: "192.168.4.3"}
	hosts, err := hostsForNetwork(network)
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 5 || slices.Contains(hosts, "192.168.4.3") ||
		slices.Contains(hosts, "192.168.4.0") || slices.Contains(hosts, "192.168.4.7") {
		t.Fatalf("hosts=%v", hosts)
	}
	for _, bad := range []string{"8.8.8.0/24", "10.0.0.0/8", "invalid", "127.0.0.0/24"} {
		_, err := hostsForNetwork(Network{CIDR: bad, Address: "192.168.4.3"})
		if err == nil {
			t.Fatalf("unsafe target accepted: %s", bad)
		}
	}
	if _, err := SelectNetwork([]Network{network}, "8.8.8.0/24"); err == nil {
		t.Fatal("user-chosen arbitrary subnet accepted")
	}
}

func TestWSDiscoveryParsesScopedNameAndValidatesReplyID(t *testing.T) {
	const id = "urn:uuid:44-55"
	body := `<?xml version="1.0"?>
<e:Envelope xmlns:e="http://www.w3.org/2003/05/soap-envelope">
<e:Header><a:RelatesTo xmlns:a="http://schemas.xmlsoap.org/ws/2004/08/addressing">` + id + `</a:RelatesTo></e:Header>
<e:Body><d:ProbeMatches xmlns:d="http://schemas.xmlsoap.org/ws/2005/04/discovery">
<d:ProbeMatch><d:Scopes>onvif://www.onvif.org/name/Front%20Gate onvif://www.onvif.org/hardware/Hikvision</d:Scopes>
<d:XAddrs>http://169.254.2.3/onvif/device_service</d:XAddrs></d:ProbeMatch></d:ProbeMatches></e:Body></e:Envelope>`
	found := parseWSDiscovery([]byte(body), id)
	if found.Service != "ONVIF WS-Discovery" || found.Name != "Front Gate" ||
		found.Vendor != "Hikvision" || !found.Confirmed {
		t.Fatalf("response: %#v", found)
	}
	if parseWSDiscovery([]byte(body), "other").Service != "" {
		t.Fatal("unrelated multicast reply accepted")
	}
	if parseWSDiscovery([]byte("<ProbeMatch><Scopes>fake</Scopes></ProbeMatch>"), id).Service != "" {
		t.Fatal("unsigned or incomplete reply accepted")
	}
}

func TestSSDPRejectsUnrelatedNetworkEquipment(t *testing.T) {
	if parseSSDPResponse([]byte("HTTP/1.1 200 OK\r\nSERVER: router/upnp\r\nST: upnp:rootdevice")).Service != "" {
		t.Fatal("router advertised as camera")
	}
	got := parseSSDPResponse([]byte("HTTP/1.1 200 OK\r\nSERVER: Dahua IPC UPnP/1.0\r\nST: urn:schemas-upnp-org:device:IPCamera:1\r\n\r\n"))
	if got.Service != "SSDP/UPnP" || got.Vendor != "Dahua" {
		t.Fatalf("SSDP device=%#v", got)
	}
}

func TestMDNSPTRDeviceDetectionAndCompressionBounds(t *testing.T) {
	query := buildMDNSQuestion([]string{"_rtsp._tcp.local"})
	if binary.BigEndian.Uint16(query[4:6]) != 1 || len(query) < 25 {
		t.Fatalf("bad mDNS query: %x", query)
	}
	packet := append([]byte(nil), query...)
	packet[2], packet[3] = 0x84, 0
	binary.BigEndian.PutUint16(packet[6:8], 1)
	// Answer name: compressed pointer to the service in the question.
	packet = append(packet, 0xc0, 0x0c, 0, 12, 0, 1, 0, 0, 0, 30)
	pointer := make([]byte, 0)
	pointer = append(pointer, byte(len("Gate Camera")))
	pointer = append(pointer, []byte("Gate Camera")...)
	pointer = append(pointer, 0xc0, 0x0c)
	var lenBytes [2]byte
	binary.BigEndian.PutUint16(lenBytes[:], uint16(len(pointer)))
	packet = append(packet, lenBytes[:]...)
	packet = append(packet, pointer...)
	result := parseMDNSResponse(packet)
	if result.Service != "mDNS" || result.Name != "Gate Camera" {
		t.Fatalf("mDNS response %#v", result)
	}
	bad := append([]byte(nil), packet...)
	bad[12], bad[13] = 0xc0, 0x0c
	if parseMDNSResponse(bad).Service != "" {
		t.Fatal("recursive compression pointer accepted")
	}
	if parseMDNSResponse(bytes.Repeat([]byte{0xff}, 20)).Service != "" {
		t.Fatal("malformed packet accepted")
	}
}

func TestProbesRecognizeRTSPWithoutAuthentication(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = server.SetDeadline(time.Now().Add(2 * time.Second))
		buf := make([]byte, 512)
		n, _ := server.Read(buf)
		if n == 0 || !strings.Contains(string(buf[:n]), "OPTIONS * RTSP/1.0") {
			return
		}
		_, _ = server.Write([]byte("RTSP/1.0 401 Unauthorized\r\nServer: Reolink RTSP\r\nWWW-Authenticate: Digest realm=\"camera\"\r\n\r\n"))
	}()
	got := probeRTSP(client, "127.0.0.1", 554)
	<-done
	if !got.Confirmed || got.Vendor != "Reolink" || got.Port != 554 {
		t.Fatalf("RTSP= %#v", got)
	}
}

func TestCameraVendorBannerClassification(t *testing.T) {
	for _, tc := range []struct{ banner, vendor string }{
		{"Server: AXIS Network Camera", "Axis"},
		{"Server: Dahua HTTP", "Dahua"},
		{"RTSP/1.0 200 OK\r\nServer: Foscam", "Foscam"},
		{"Server: Xiongmai XM", "Xiongmai"},
		{"Server: generic router", ""},
	} {
		if got := vendorFromText(tc.banner); got != tc.vendor {
			t.Fatalf("%q => %q", tc.banner, got)
		}
	}
}

func TestScanRejectsUnsafeCIDRBeforeNetworking(t *testing.T) {
	for _, network := range []Network{{CIDR: "0.0.0.0/0"}, {CIDR: "192.168.0.0/16"}, {CIDR: "8.8.8.0/24"}} {
		if _, err := Scan(context.Background(), network); err == nil {
			t.Fatalf("scan accepted arbitrary target %q", network.CIDR)
		}
	}
}
