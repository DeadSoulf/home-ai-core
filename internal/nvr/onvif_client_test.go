package nvr

import (
	"io"
	"net/http"
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


type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestONVIFProfilesRetriesWithHTTPDigest(t *testing.T) {
	const challenge = `Digest realm="IP Camera", nonce="abcdef0123456789", qop="auth", algorithm=MD5`
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(request.Body)
		if !strings.HasPrefix(request.Header.Get("Authorization"), "Digest ") {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Header: http.Header{
					"WWW-Authenticate": []string{challenge},
				},
				Body: io.NopCloser(strings.NewReader("")),
				Request: request,
			}, nil
		}

		var response string
		switch {
		case strings.Contains(string(body), "GetCapabilities"):
			response = `<Envelope><Body><GetCapabilitiesResponse><Capabilities><Media XAddr="http://192.168.1.40/onvif/media_service"/></Capabilities></GetCapabilitiesResponse></Body></Envelope>`
		case strings.Contains(string(body), "GetProfiles"):
			response = `<Envelope><Body><GetProfilesResponse><Profiles token="main"><Name>Main</Name><VideoEncoderConfiguration><Encoding>H264</Encoding><Resolution><Width>1920</Width><Height>1080</Height></Resolution></VideoEncoderConfiguration></Profiles></GetProfilesResponse></Body></Envelope>`
		case strings.Contains(string(body), "GetStreamUri"):
			response = `<Envelope><Body><GetStreamUriResponse><MediaUri><Uri>rtsp://192.168.1.40:554/Streaming/Channels/101</Uri></MediaUri></GetStreamUriResponse></Body></Envelope>`
		default:
			t.Fatalf("unexpected SOAP request: %s", body)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{"Content-Type": []string{"application/soap+xml"}},
			Body: io.NopCloser(strings.NewReader(response)),
			Request: request,
		}, nil
	})

	client := NewSOAPONVIFClient()
	client.client = &http.Client{Transport: transport}
	profiles, err := client.Profiles(
		t.Context(),
		"http://192.168.1.40/onvif/device_service",
		CameraCredential{Username: "onvif-user", Password: "secret"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 || profiles[0].StreamURI != "rtsp://192.168.1.40:554/Streaming/Channels/101" {
		t.Fatalf("profiles = %#v", profiles)
	}
}

func TestHTTPDigestAuthorizationDoesNotLeakPassword(t *testing.T) {
	header, err := buildHTTPDigestAuthorization(
		`Digest realm="IP Camera", nonce="abcdef", qop="auth", algorithm=MD5`,
		"viewer",
		"super-secret",
		"POST",
		"/onvif/device_service",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(header, "Digest ") {
		t.Fatalf("authorization = %q", header)
	}
	if strings.Contains(header, "super-secret") {
		t.Fatalf("authorization leaked password: %s", header)
	}
	if !strings.Contains(header, `username="viewer"`) ||
		!strings.Contains(header, `uri="/onvif/device_service"`) ||
		!strings.Contains(header, "qop=auth") {
		t.Fatalf("unexpected authorization: %s", header)
	}
}
