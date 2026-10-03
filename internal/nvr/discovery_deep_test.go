package nvr

import (
	"encoding/binary"
	"net"
	"strings"
	"testing"
)

func TestDeepCameraTargetsExplicitCIDR(t *testing.T) {
	targets, err := deepCameraTargets("192.168.44.0/24")
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 254 {
		t.Fatalf("targets = %d, want 254", len(targets))
	}
	if targets[0].String() != "192.168.44.1" || targets[len(targets)-1].String() != "192.168.44.254" {
		t.Fatalf("unexpected target bounds: %s .. %s", targets[0], targets[len(targets)-1])
	}
}

func TestDeepCameraTargetsRejectsUnsafeRanges(t *testing.T) {
	for _, value := range []string{
		"8.8.8.0/24",
		"192.168.0.0/19",
		"192.168.1.0/31",
		"not-a-cidr",
	} {
		if _, err := deepCameraTargets(value); err == nil {
			t.Fatalf("expected %q to be rejected", value)
		}
	}
}

func TestParseHikvisionSADPResponse(t *testing.T) {
	raw := []byte(`<?xml version="1.0" encoding="utf-8"?>
<ProbeMatch>
  <Types>inquiry</Types>
  <DeviceType>DS-2CD2143G0-I</DeviceType>
  <DeviceDescription>DS-2CD2143G0-I</DeviceDescription>
  <IPv4Address>192.168.10.64</IPv4Address>
  <MAC>4c-bd-8f-12-34-56</MAC>
  <HttpPort>80</HttpPort>
  <CommandPort>8000</CommandPort>
  <RtspPort>554</RtspPort>
</ProbeMatch>`)
	device, ok := parseHikvisionSADPResponse(raw, net.ParseIP("192.168.10.64"))
	if !ok {
		t.Fatal("Hikvision SADP response was not recognized")
	}
	if device.Vendor != "Hikvision" || device.IP != "192.168.10.64" {
		t.Fatalf("device = %#v", device)
	}
	if device.MAC != "4C:BD:8F:12:34:56" {
		t.Fatalf("MAC = %q", device.MAC)
	}
	if device.RTSPAddressHint != "rtsp://192.168.10.64/" {
		t.Fatalf("RTSP hint = %q", device.RTSPAddressHint)
	}
	if !containsDiscoverySource(device.Sources, "hikvision:sadp") {
		t.Fatalf("sources = %#v", device.Sources)
	}
}

func TestDahuaDHIPDiscoveryProbeFrame(t *testing.T) {
	raw := dahuaDHIPDiscoveryProbe()
	if len(raw) <= 32 {
		t.Fatalf("probe too short: %d", len(raw))
	}
	if got := binary.LittleEndian.Uint32(raw[0:4]); got != 32 {
		t.Fatalf("header size = %#x", got)
	}
	if got := binary.LittleEndian.Uint32(raw[4:8]); got != 0x50494844 {
		t.Fatalf("magic = %#x", got)
	}
	if !strings.Contains(string(raw[32:]), "DHDiscover.search") {
		t.Fatalf("unexpected body: %s", raw[32:])
	}
}

func TestParseDahuaDHIPDiscovery(t *testing.T) {
	body := []byte(`{"params":{"deviceInfo":{"IPv4Address":"192.168.10.80","Mac":"AA:BB:CC:DD:EE:FF","DeviceType":"IPC-HFW1230S","HttpPort":"80","RtspPort":"554","Port":"5000"}}}`)
	raw := make([]byte, 32+len(body))
	binary.LittleEndian.PutUint32(raw[0:4], 32)
	binary.LittleEndian.PutUint32(raw[4:8], 0x50494844)
	binary.LittleEndian.PutUint32(raw[16:20], uint32(len(body)))
	binary.LittleEndian.PutUint32(raw[24:28], uint32(len(body)))
	copy(raw[32:], body)

	device, ok := parseDahuaDHIPDiscovery(raw, net.ParseIP("192.168.10.80"))
	if !ok {
		t.Fatal("Dahua DHIP response was not recognized")
	}
	if device.Vendor != "Dahua" || device.Model != "IPC-HFW1230S" {
		t.Fatalf("device = %#v", device)
	}
	if device.RTSPAddressHint != "rtsp://192.168.10.80/" {
		t.Fatalf("RTSP hint = %q", device.RTSPAddressHint)
	}
	if !containsDiscoverySource(device.Sources, "dahua:dhip") {
		t.Fatalf("sources = %#v", device.Sources)
	}
}

func TestParseCameraSSDPResponse(t *testing.T) {
	raw := []byte("HTTP/1.1 200 OK\r\n" +
		"SERVER: AXIS Camera/1.0\r\n" +
		"ST: urn:schemas-upnp-org:device:Basic:1\r\n" +
		"USN: uuid:axis-camera\r\n" +
		"LOCATION: http://192.168.1.90:80/rootDesc.xml\r\n\r\n")
	device, ok := parseCameraSSDPResponse(raw, net.ParseIP("192.168.1.90"))
	if !ok {
		t.Fatal("camera SSDP response was not recognized")
	}
	if device.Vendor != "Axis" || device.IP != "192.168.1.90" {
		t.Fatalf("device = %#v", device)
	}
	if !containsDiscoverySource(device.Sources, "ssdp") {
		t.Fatalf("sources = %#v", device.Sources)
	}
}

func TestParseCameraSSDPRejectsGenericUPnP(t *testing.T) {
	raw := []byte("HTTP/1.1 200 OK\r\n" +
		"SERVER: Generic Router\r\n" +
		"ST: urn:schemas-upnp-org:device:InternetGatewayDevice:1\r\n" +
		"LOCATION: http://192.168.1.1/rootDesc.xml\r\n\r\n")
	if device, ok := parseCameraSSDPResponse(raw, net.ParseIP("192.168.1.1")); ok {
		t.Fatalf("generic UPnP device accepted: %#v", device)
	}
}

func containsDiscoverySource(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
