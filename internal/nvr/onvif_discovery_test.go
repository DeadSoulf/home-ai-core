package nvr

import (
	"net"
	"strings"
	"testing"
)

func TestParseWSDiscoveryResponseNormalizesEndpointToSenderIP(t *testing.T) {
	raw := []byte(`<?xml version="1.0"?>
	<e:Envelope xmlns:e="http://www.w3.org/2003/05/soap-envelope"
	 xmlns:d="http://schemas.xmlsoap.org/ws/2005/04/discovery"
	 xmlns:a="http://schemas.xmlsoap.org/ws/2004/08/addressing">
	  <e:Body>
	    <d:ProbeMatches>
	      <d:ProbeMatch>
	        <a:EndpointReference><a:Address>urn:uuid:test-camera</a:Address></a:EndpointReference>
	        <d:Scopes>onvif://www.onvif.org/name/Front%20Door onvif://www.onvif.org/type/video_encoder</d:Scopes>
	        <d:XAddrs>http://camera-host:8899/onvif/device_service</d:XAddrs>
	      </d:ProbeMatch>
	    </d:ProbeMatches>
	  </e:Body>
	</e:Envelope>`)
	devices := parseWSDiscoveryResponse(raw, net.ParseIP("192.168.10.44"))
	if len(devices) != 1 {
		t.Fatalf("devices = %#v", devices)
	}
	if devices[0].Name != "Front Door" || devices[0].IP != "192.168.10.44" {
		t.Fatalf("device = %#v", devices[0])
	}
	if devices[0].Address != "http://192.168.10.44:8899/onvif/device_service" {
		t.Fatalf("address = %q", devices[0].Address)
	}
}

func TestValidateLocalONVIFEndpointRejectsPublicAndCredentialedURLs(t *testing.T) {
	for _, value := range []string{
		"https://8.8.8.8/onvif/device_service",
		"http://user:secret@192.168.1.10/onvif/device_service",
		"http://127.0.0.1/onvif/device_service",
		"http://example.com/onvif/device_service",
	} {
		if _, _, err := validateLocalONVIFEndpoint(value); err == nil {
			t.Fatalf("unsafe ONVIF endpoint accepted: %s", value)
		}
	}
	parsed, ip, err := validateLocalONVIFEndpoint("http://192.168.1.10/onvif/device_service")
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Hostname() != "192.168.1.10" || !ip.Equal(net.ParseIP("192.168.1.10")) {
		t.Fatalf("validated endpoint = %v %v", parsed, ip)
	}
}

func TestWSDiscoveryProbeTargetsONVIFNetworkVideoTransmitter(t *testing.T) {
	probe, err := wsDiscoveryProbe()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(probe), "NetworkVideoTransmitter") ||
		!strings.Contains(string(probe), "/discovery/Probe") {
		t.Fatalf("unexpected WS-Discovery probe: %s", probe)
	}
}

func TestDiscoveryBindIPsFallsBackToWildcard(t *testing.T) {
	got := discoveryBindIPs(nil, nil)
	if len(got) != 1 || !got[0].Equal(net.IPv4zero) {
		t.Fatalf("fallback = %#v, want 0.0.0.0", got)
	}

	want := net.ParseIP("192.168.1.20").To4()
	got = discoveryBindIPs([]net.IP{want}, nil)
	if len(got) != 1 || !got[0].Equal(want) {
		t.Fatalf("specific bind IPs changed: %#v", got)
	}
}
