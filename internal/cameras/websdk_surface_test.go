package cameras

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebSDKFunctionCatalogContainsEntireV331Surface(t *testing.T) {
	functions := WebSDKFunctions()
	if len(functions) != 79 {
		t.Fatalf("function count = %d, want 79", len(functions))
	}
	seen := map[string]bool{}
	for _, function := range functions {
		if function.Name == "" || function.Backend == "" || function.Status == "" {
			t.Fatalf("invalid function descriptor: %#v", function)
		}
		if seen[function.Name] {
			t.Fatalf("duplicate function %q", function.Name)
		}
		seen[function.Name] = true
	}
	for _, name := range []string{
		"I_Login",
		"I_GetDeviceInfo",
		"I_StartRealPlay",
		"I_PTZControl",
		"I_RecordSearch",
		"I_ExportDeviceConfig",
		"I_StartUpgrade",
		"I_SendHTTPRequest",
		"I_DeviceCapturePic",
		"I_GetTextOverlay",
	} {
		if !seen[name] {
			t.Fatalf("missing WebSDK function %q", name)
		}
	}
}

func TestValidateWebSDKPath(t *testing.T) {
	for _, value := range []string{
		"/ISAPI/System/deviceInfo",
		"/SDK/capabilities",
		"/PSIA/Custom/SelfExt/ContentMgmt/ZeroStreaming/channels/101",
	} {
		if _, err := validateWebSDKPath(value); err != nil {
			t.Fatalf("%s: %v", value, err)
		}
	}
	for _, value := range []string{
		"http://127.0.0.1/ISAPI/System/deviceInfo",
		"//127.0.0.1/ISAPI/System/deviceInfo",
		"/api/v1/system",
		"/ISAPI/../../etc/passwd",
	} {
		if _, err := validateWebSDKPath(value); err == nil {
			t.Fatalf("%s: expected rejection", value)
		}
	}
}

func TestSendWebSDKRequestUsesDigestAndReturnsXML(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/ISAPI/System/deviceInfo" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Digest ") {
			w.Header().Set("WWW-Authenticate", `Digest realm="camera", nonce="abcdef", algorithm=MD5, qop="auth"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte("<DeviceInfo><model>DS-2CD</model></DeviceInfo>"))
	}))
	defer server.Close()

	request := WebSDKRawRequest{
		Address:  "127.0.0.1",
		Port:     80,
		Username: "admin",
		Password: "secret",
		Method:   http.MethodGet,
		Path:     "/ISAPI/System/deviceInfo",
	}
	// Use the httptest transport/base URL directly because validateWebSDKRequest
	// intentionally rejects loopback targets in production.
	client := &webSDKClient{
		client:   server.Client(),
		baseURL:  server.URL,
		username: request.Username,
		password: request.Password,
	}
	status, contentType, payload, err := client.requestBytes(
		context.Background(),
		request.Method,
		request.Path,
		"",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || status != http.StatusOK || contentType != "application/xml" {
		t.Fatalf("calls=%d status=%d contentType=%q", calls, status, contentType)
	}
	if !strings.Contains(string(payload), "DS-2CD") {
		t.Fatalf("payload = %q", payload)
	}
}
