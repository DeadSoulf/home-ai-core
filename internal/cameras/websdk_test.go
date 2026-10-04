package cameras

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseWebSDKDeviceAndChannels(t *testing.T) {
	device, err := parseWebSDKDeviceInfo([]byte(`<?xml version="1.0"?>
<DeviceInfo>
  <deviceName>Front NVR</deviceName>
  <deviceID>88</deviceID>
  <deviceType>NVR</deviceType>
  <model>DS-7608NI-K2</model>
  <serialNumber>DS-TEST-001</serialNumber>
  <macAddress>00:11:22:33:44:55</macAddress>
  <firmwareVersion>V4.72.109</firmwareVersion>
  <firmwareReleasedDate>2024-01-01</firmwareReleasedDate>
</DeviceInfo>`))
	if err != nil {
		t.Fatal(err)
	}
	if device.Model != "DS-7608NI-K2" || device.SerialNumber != "DS-TEST-001" {
		t.Fatalf("device = %#v", device)
	}

	analog, err := parseWebSDKAnalogChannels([]byte(`<VideoInputChannelList>
  <VideoInputChannel><id>1</id><inputPort>1</inputPort><name>Analog 1</name><videoFormat>PAL</videoFormat></VideoInputChannel>
</VideoInputChannelList>`))
	if err != nil {
		t.Fatal(err)
	}
	if len(analog) != 1 || analog[0].ID != "1" || analog[0].Kind != "analog" {
		t.Fatalf("analog = %#v", analog)
	}

	names, err := parseWebSDKDigitalChannels([]byte(`<InputProxyChannelList>
  <InputProxyChannel><id>3</id><name>Gate</name></InputProxyChannel>
</InputProxyChannelList>`))
	if err != nil {
		t.Fatal(err)
	}
	digital, err := parseWebSDKDigitalChannelStatus([]byte(`<InputProxyChannelStatusList>
  <InputProxyChannelStatus>
    <id>3</id>
    <sourceInputPortDescriptor>
      <proxyProtocol>HIKVISION</proxyProtocol>
      <ipAddress>192.168.1.64</ipAddress>
      <managePortNo>8000</managePortNo>
      <srcInputPort>1</srcInputPort>
      <streamType>main</streamType>
      <online>true</online>
    </sourceInputPortDescriptor>
  </InputProxyChannelStatus>
</InputProxyChannelStatusList>`), names)
	if err != nil {
		t.Fatal(err)
	}
	if len(digital) != 1 || digital[0].Name != "Gate" || digital[0].Online == nil || !*digital[0].Online {
		t.Fatalf("digital = %#v", digital)
	}
}

func TestParseWebSDKPorts(t *testing.T) {
	ports, err := parseWebSDKPorts([]byte(`<AdminAccessProtocolList>
  <AdminAccessProtocol><protocol>http</protocol><portNo>80</portNo></AdminAccessProtocol>
  <AdminAccessProtocol><protocol>rtsp</protocol><portNo>554</portNo></AdminAccessProtocol>
  <AdminAccessProtocol><protocol>dev_manage</protocol><portNo>8000</portNo></AdminAccessProtocol>
</AdminAccessProtocolList>`))
	if err != nil {
		t.Fatal(err)
	}
	if ports.HTTPPort != 80 || ports.RTSPPort != 554 || ports.DevicePort != 8000 {
		t.Fatalf("ports = %#v", ports)
	}
}

func TestWebSDKClientUsesDigestAuthentication(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Digest ") {
			w.Header().Set("WWW-Authenticate", `Digest realm="camera", nonce="abcdef", algorithm=MD5, qop="auth"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if !strings.Contains(r.Header.Get("Authorization"), `username="admin"`) {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte("<DeviceInfo><model>DS-2CD</model></DeviceInfo>"))
	}))
	defer server.Close()

	client := &webSDKClient{
		client:   server.Client(),
		baseURL:  server.URL,
		username: "admin",
		password: "secret",
	}
	payload, err := client.get(context.Background(), "/ISAPI/System/deviceInfo")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || !strings.Contains(string(payload), "DS-2CD") {
		t.Fatalf("calls=%d payload=%q", calls, payload)
	}
}

func TestWebSDKProbeRejectsPublicAddress(t *testing.T) {
	service := NewServiceWithRuntime(&fakeSDKRuntime{})
	_, err := service.ProbeWebSDK(context.Background(), WebSDKProbeRequest{
		Address:  "8.8.8.8",
		Port:     80,
		Username: "admin",
		Password: "secret",
	})
	if !errorsIsInvalidTarget(err) {
		t.Fatalf("error = %v, want invalid target", err)
	}
}

func errorsIsInvalidTarget(err error) bool {
	return err != nil && strings.Contains(err.Error(), ErrInvalidTarget.Error())
}
