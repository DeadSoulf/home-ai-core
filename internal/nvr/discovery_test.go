package nvr

import (
	"net"
	"strings"
	"testing"
)

func TestDetectCameraVendor(t *testing.T) {
	cases := map[string]string{
		"HIKVISION WebComponents":             "Hikvision",
		"Dahua Technology":                    "Dahua",
		"UNIVIEW Network Camera":              "Uniview",
		"Wisenet Hanwha Vision":               "Hanwha",
		"AXIS Communications video":           "Axis",
		"Bosch Security Systems video device": "Bosch",
	}
	for input, want := range cases {
		if got := detectCameraVendor(input); got != want {
			t.Fatalf("detectCameraVendor(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestClassifyDiscoveryDevice(t *testing.T) {
	camera := &CameraDiscoveryDevice{
		Name:         "Front Camera",
		ONVIFAddress: "http://192.168.1.20/onvif/device_service",
		Services: []CameraDiscoveryService{
			{Protocol: "rtsp", Port: 554},
		},
	}
	classifyDiscoveryDevice(camera)
	if camera.DeviceType != "camera" || camera.Confidence != "high" {
		t.Fatalf("camera classification = %#v", camera)
	}

	recorder := &CameraDiscoveryDevice{
		Name:         "Garage NVR",
		ONVIFAddress: "http://192.168.1.30/onvif/device_service",
	}
	classifyDiscoveryDevice(recorder)
	if recorder.DeviceType != "recorder" {
		t.Fatalf("recorder classification = %#v", recorder)
	}
}

func TestONVIFScopeValue(t *testing.T) {
	scopes := []string{
		"onvif://www.onvif.org/name/Front%20Door",
		"onvif://www.onvif.org/hardware/DS-2CD2143G0-I",
	}
	if got := onvifScopeValue(scopes, []string{"/hardware/"}); got != "DS-2CD2143G0-I" {
		t.Fatalf("hardware scope = %q", got)
	}
}

func TestReadRTSPResponsePrefix(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		buffer := make([]byte, 2048)
		_, _ = server.Read(buffer)
		_, _ = server.Write([]byte("RTSP/1.0 401 Unauthorized\r\nCSeq: 1\r\n\r\n"))
	}()

	if !probeRTSPConnection(client, "192.168.1.40", 554) {
		t.Fatal("authenticated RTSP endpoint was not recognized")
	}
	<-done
}

func TestVendorFingerprintsDoNotMatchGenericWeb(t *testing.T) {
	if got := detectCameraVendor(strings.Repeat("generic web server ", 4)); got != "" {
		t.Fatalf("generic web server matched vendor %q", got)
	}
}
