package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DeadSoulf/home-ai-core/internal/modules"
	"github.com/DeadSoulf/home-ai-core/internal/security"
)

type nvrRoundTripFunc func(*http.Request) (*http.Response, error)

func (f nvrRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func nvrTestService(enabled bool) fakeModules {
	status := "disabled"
	if enabled {
		status = "enabled"
	}
	return fakeModules{items: []modules.Registered{{
		Manifest: modules.Manifest{ID: "nvr.module", Name: "NVR", Version: "1.1.2"},
		Status:   status,
	}}}
}

func nvrTestUpstream(t *testing.T, body string) *server {
	t.Helper()
	return &server{
		modules:     nvrTestService(true),
		nvrEndpoint: func(context.Context) (string, error) { return "172.19.0.4", nil },
		nvrTransport: nvrRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Host != "172.19.0.4:8090" {
				t.Errorf("unsafe endpoint %s", req.URL.String())
			}
			if req.Header.Get("Cookie") != "" || req.Header.Get("Authorization") != "" {
				t.Error("browser credentials forwarded upstream")
			}
			return &http.Response{
				StatusCode: 200,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		}),
	}
}

func TestNVRLegacyManifestGetsBuiltInSidebarNavigation(t *testing.T) {
	s := &server{modules: nvrTestService(true)}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/modules/navigation", nil)
	w := httptest.NewRecorder()
	s.moduleNavigation(w, req, security.Actor{Permissions: []string{"modules.read"}}, authBearer)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"route":"/modules/nvr.module"`) {
		t.Fatalf("installed NVR missing from navigation: %d %s", w.Code, w.Body.String())
	}
	s.modules = nvrTestService(false)
	w = httptest.NewRecorder()
	s.moduleNavigation(w, req, security.Actor{Permissions: []string{"modules.read"}}, authBearer)
	if !strings.Contains(w.Body.String(), `"route":"/modules/nvr.module"`) {
		t.Fatalf("stopped installed module disappeared from menu: %s", w.Body.String())
	}
}

func TestNVRCameraResponseStripsRTSPCredentials(t *testing.T) {
	s := nvrTestUpstream(t, `{"ok":true,"cameras":[{"id":"cam-1","name":"Gate","source_type":"rtsp","rtsp_url":"rtsp://user:secret@camera/private","password":"secret","recording":true}]}`)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nvr/cameras", nil)
	w := httptest.NewRecorder()
	s.nvrCameras(w, req, security.Actor{}, authBearer)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Gate") ||
		strings.Contains(w.Body.String(), "secret") || strings.Contains(w.Body.String(), "rtsp_url") {
		t.Fatalf("credential exposure or invalid response: %d %s", w.Code, w.Body.String())
	}
}

func TestNVRRecordingsRewriteToAuthenticatedMedia(t *testing.T) {
	s := nvrTestUpstream(t, `{"ok":true,"recordings":[{"id":7,"camera_id":"cam1","path":"recordings/cam1/20261017_080000.mp4","url":"/output/recordings/cam1/20261017_080000.mp4"}]}`)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nvr/recordings", nil)
	w := httptest.NewRecorder()
	s.nvrRecordings(w, req, security.Actor{}, authBearer)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "/api/v1/nvr/output/recordings/cam1/20261017_080000.mp4") {
		t.Fatalf("unauthenticated media link: %d %s", w.Code, w.Body.String())
	}
}

func TestNVRMediaTraversalCannotReachSDKOrSecrets(t *testing.T) {
	for _, path := range []string{
		"recordings/../secrets.mp4", "logs/sdk.log", "hls/ch/../private.ts",
		"recordings/camera/private.json", "recordings/cam", "recordings/cam/../../etc/passwd",
	} {
		if validNVROutputPath(path) {
			t.Fatalf("unsafe media path accepted: %q", path)
		}
	}
	for _, path := range []string{"recordings/cam/20261017_101500.mp4", "hls/ch1/index.m3u8", "hls/ch1/segment001.ts"} {
		if !validNVROutputPath(path) {
			t.Fatalf("safe media path rejected: %q", path)
		}
	}
}

func TestNVRModuleStateAndPermissions(t *testing.T) {
	disabled := uiAPI(defaultFakeSecurity(), &uiJobService{}, uiModuleService{fakeModules: nvrTestService(false)})
	if got := uiRequest(disabled, "GET", "/api/v1/nvr/status", ""); got.Code != 503 {
		t.Fatalf("disabled NVR exposed: %d", got.Code)
	}
	sec := defaultFakeSecurity()
	sec.actor.Permissions = []string{"security.self.read"}
	active := uiAPI(sec, &uiJobService{}, uiModuleService{fakeModules: nvrTestService(true)})
	if got := uiRequest(active, "GET", "/api/v1/nvr/status", ""); got.Code != 403 {
		t.Fatalf("viewer without modules.read allowed: %d", got.Code)
	}
	sec.actor.Permissions = []string{"modules.read"}
	readOnly := uiAPI(sec, &uiJobService{}, uiModuleService{fakeModules: nvrTestService(true)})
	if got := uiRequest(readOnly, "POST", "/api/v1/nvr/cameras", `{"name":"Test"}`); got.Code != 403 {
		t.Fatalf("read-only viewer changed NVR: %d", got.Code)
	}
}

func TestNVRMutationsRejectMissingCookieCSRF(t *testing.T) {
	s := &server{modules: nvrTestService(true)}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/nvr/cameras", strings.NewReader(`{"name":"front","source_type":"rtsp","rtsp_url":"rtsp://x"}`))
	w := httptest.NewRecorder()
	s.nvrAddCamera(w, req, security.Actor{}, authCookie)
	if w.Code != http.StatusForbidden {
		t.Fatalf("missing CSRF accepted: %d", w.Code)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/nvr/cameras/cam1/start", nil)
	req.SetPathValue("cameraID", "cam1")
	req.SetPathValue("action", "start")
	w = httptest.NewRecorder()
	s.nvrControlCamera(w, req, security.Actor{}, authCookie)
	if w.Code != http.StatusForbidden {
		t.Fatalf("missing control CSRF accepted: %d", w.Code)
	}
}

func TestNVRUntrustedResolverRejected(t *testing.T) {
	s := &server{nvrEndpoint: func(context.Context) (string, error) { return "127.0.0.1", nil }}
	if _, err := s.nvrAddress(context.Background()); err == nil {
		t.Fatal("loopback SSRF destination accepted")
	}
	s.nvrEndpoint = func(context.Context) (string, error) { return "8.8.8.8", nil }
	if _, err := s.nvrAddress(context.Background()); err == nil {
		t.Fatal("public address SSRF destination accepted")
	}
}

func TestNVRStatusJSONDoesNotLeakDatabasePath(t *testing.T) {
	s := nvrTestUpstream(t, `{"ok":true,"service":"nvr","database":"/data/nvr.db","cameras":2}`)
	w := httptest.NewRecorder()
	s.nvrStatus(w, httptest.NewRequest(http.MethodGet, "/api/v1/nvr/status", nil), security.Actor{}, authBearer)
	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if _, ok := result["database"]; ok {
		t.Fatal("private NVR database path leaked")
	}
}

func TestNVRArchiveSupportsRangesAndRejectsEscapingSymlinks(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "recordings", "cam1")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "sample.mp4"), []byte("abcdefghij"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := nvrTestUpstream(t, "{}")
	s.nvrDataRoot = dir
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nvr/output/recordings/cam1/sample.mp4", nil)
	req.SetPathValue("path", "recordings/cam1/sample.mp4")
	req.Header.Set("Range", "bytes=2-5")
	w := httptest.NewRecorder()
	s.nvrOutput(w, req, security.Actor{}, authBearer)
	if w.Code != http.StatusPartialContent || w.Body.String() != "cdef" {
		t.Fatalf("range status %d body %q", w.Code, w.Body.String())
	}
	external := filepath.Join(t.TempDir(), "secret.mp4")
	if err := os.WriteFile(external, []byte("DO_NOT_EXPOSE"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(target, "escaped.mp4")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/nvr/output/recordings/cam1/escaped.mp4", nil)
	req.SetPathValue("path", "recordings/cam1/escaped.mp4")
	w = httptest.NewRecorder()
	s.nvrOutput(w, req, security.Actor{}, authBearer)
	if w.Code == http.StatusOK || strings.Contains(w.Body.String(), "DO_NOT_EXPOSE") {
		t.Fatalf("root escape accepted: %d %q", w.Code, w.Body.String())
	}
}
