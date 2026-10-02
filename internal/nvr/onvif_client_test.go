package nvr

import (
	"strings"
	"testing"
)

func TestParseONVIFProfilesAndStreamURI(t *testing.T) {
	profilesXML := []byte(`<?xml version="1.0"?>
	<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
	 xmlns:trt="http://www.onvif.org/ver10/media/wsdl"
	 xmlns:tt="http://www.onvif.org/ver10/schema">
	  <s:Body><trt:GetProfilesResponse>
	    <trt:Profiles token="main">
	      <tt:Name>Main Stream</tt:Name>
	      <tt:VideoEncoderConfiguration>
	        <tt:Encoding>H264</tt:Encoding>
	        <tt:Resolution><tt:Width>1920</tt:Width><tt:Height>1080</tt:Height></tt:Resolution>
	        <tt:RateControl><tt:FrameRateLimit>25</tt:FrameRateLimit><tt:BitrateLimit>4096</tt:BitrateLimit></tt:RateControl>
	      </tt:VideoEncoderConfiguration>
	      <tt:AudioEncoderConfiguration/>
	    </trt:Profiles>
	    <trt:Profiles token="sub">
	      <tt:Name>Sub Stream</tt:Name>
	      <tt:VideoEncoderConfiguration>
	        <tt:Encoding>H264</tt:Encoding>
	        <tt:Resolution><tt:Width>640</tt:Width><tt:Height>360</tt:Height></tt:Resolution>
	        <tt:RateControl><tt:FrameRateLimit>10</tt:FrameRateLimit><tt:BitrateLimit>512</tt:BitrateLimit></tt:RateControl>
	      </tt:VideoEncoderConfiguration>
	    </trt:Profiles>
	  </trt:GetProfilesResponse></s:Body>
	</s:Envelope>`)
	profiles, err := parseONVIFProfiles(profilesXML)
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 2 {
		t.Fatalf("profiles = %#v", profiles)
	}
	if profiles[0].Token != "main" || profiles[0].Width != 1920 ||
		profiles[0].BitrateBPS != 4_096_000 || !profiles[0].HasAudio {
		t.Fatalf("main profile = %#v", profiles[0])
	}

	streamXML := []byte(`<Envelope><Body><GetStreamUriResponse><MediaUri><Uri>rtsp://user:secret@camera.local:554/main</Uri></MediaUri></GetStreamUriResponse></Body></Envelope>`)
	uri, err := parseONVIFStreamURI(streamXML)
	if err != nil {
		t.Fatal(err)
	}
	safe, err := sanitizeONVIFRTSPURI(uri, []byte{192, 168, 1, 40})
	if err != nil {
		t.Fatal(err)
	}
	if safe != "rtsp://192.168.1.40:554/main" {
		t.Fatalf("safe stream URI = %q", safe)
	}
	if strings.Contains(safe, "user") || strings.Contains(safe, "secret") {
		t.Fatalf("stream URI leaked credentials: %q", safe)
	}
}

func TestParseONVIFMediaXAddr(t *testing.T) {
	raw := []byte(`<Envelope><Body><GetCapabilitiesResponse><Capabilities><Media XAddr="http://192.168.1.40/onvif/media_service"/></Capabilities></GetCapabilitiesResponse></Body></Envelope>`)
	address, err := parseONVIFMediaXAddr(raw)
	if err != nil {
		t.Fatal(err)
	}
	if address != "http://192.168.1.40/onvif/media_service" {
		t.Fatalf("media address = %q", address)
	}
}

func TestONVIFSecurityHeaderDoesNotUsePlaintextPassword(t *testing.T) {
	header, err := onvifSecurityHeader(CameraCredential{Username: "viewer", Password: "super-secret"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(header, "super-secret") {
		t.Fatalf("WS-Security header leaked plaintext password: %s", header)
	}
	if !strings.Contains(header, "PasswordDigest") || !strings.Contains(header, "viewer") {
		t.Fatalf("unexpected WS-Security header: %s", header)
	}
}
