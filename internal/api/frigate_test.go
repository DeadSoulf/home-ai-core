package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/frigate"
	"github.com/DeadSoulf/home-ai-core/internal/modules"
	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/state"
)

type fakeFrigateUpstream struct {
	candidate                   frigate.Candidate
	discoverErr, resolveErr     error
	discoverCalls, resolveCalls int
}

func (f *fakeFrigateUpstream) Discover(context.Context, string) (frigate.Discovery, error) {
	f.discoverCalls++
	return frigate.Discovery{LatestVersion: f.candidate.Version, SupportedRange: frigate.SupportedVersions, Candidate: &f.candidate, CheckedAt: time.Now().UTC()}, f.discoverErr
}
func (f *fakeFrigateUpstream) Resolve(context.Context, string, string) (frigate.Candidate, error) {
	f.resolveCalls++
	return f.candidate, f.resolveErr
}
func frigateAPICandidate() frigate.Candidate {
	return frigate.Candidate{Version: "0.18.2", Architecture: runtime.GOARCH, Image: frigate.ImageRepository + "@sha256:" + strings.Repeat("a", 64), Digest: "sha256:" + strings.Repeat("a", 64)}
}
func frigateAPIModules(enabled bool) fakeModules {
	status := "disabled"
	if enabled {
		status = "enabled"
	}
	return fakeModules{items: []modules.Registered{{Manifest: modules.FrigateManifest(frigateAPICandidate()), Status: status}}}
}
func TestFrigateDiscoveryCachesAndNeverInstalls(t *testing.T) {
	upstream := &fakeFrigateUpstream{candidate: frigateAPICandidate()}
	jobs := &uiJobService{}
	s := &server{frigateUpstream: upstream, jobs: jobs}
	first := s.frigateReleaseStatus(context.Background(), false)
	second := s.frigateReleaseStatus(context.Background(), true)
	if first.Candidate == nil || second.Candidate == nil || upstream.discoverCalls != 1 || len(jobs.jobs) != 0 {
		t.Fatal("discovery did not cache, or performed a mutation")
	}
	upstream.discoverErr = errors.New("network unavailable with private transport detail")
	s.frigateCatalog.lastAttempt = time.Now().Add(-31 * time.Second)
	stale := s.frigateReleaseStatus(context.Background(), true)
	if !stale.Stale || stale.Candidate == nil || stale.Candidate.Image != first.Candidate.Image || len(jobs.jobs) != 0 || strings.Contains(stale.Error, "private transport") {
		t.Fatalf("unsafe stale cache: %#v", stale)
	}
}
func TestFrigateMutationsRequireExactFreshApproval(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux deployment policy")
	}
	candidate := frigateAPICandidate()
	for _, test := range []struct {
		name, body string
		source     authSource
		resolveErr error
		want       int
	}{
		{name: "missing", body: "{}", source: authBearer, want: 400},
		{name: "malformed", body: "{", source: authBearer, want: 400},
		{name: "unknown_field", body: `{"expected_version":"0.18.2","expected_image":"` + candidate.Image + `","registry":"evil.test"}`, source: authBearer, want: 400},
		{name: "csrf", body: `{"expected_version":"0.18.2","expected_image":"` + candidate.Image + `"}`, source: authCookie, want: 403},
		{name: "digest_changed", body: `{"expected_version":"0.18.2","expected_image":"` + frigate.ImageRepository + `@sha256:` + strings.Repeat("b", 64) + `"}`, source: authBearer, want: 409},
		{name: "fresh_failure", body: `{"expected_version":"0.18.2","expected_image":"` + candidate.Image + `"}`, source: authBearer, resolveErr: errors.New("offline"), want: 503},
		{name: "approved", body: `{"expected_version":"0.18.2","expected_image":"` + candidate.Image + `"}`, source: authBearer, want: 202},
	} {
		t.Run(test.name, func(t *testing.T) {
			jobs := &uiJobService{}
			upstream := &fakeFrigateUpstream{candidate: candidate, resolveErr: test.resolveErr}
			s := &server{jobs: jobs, security: defaultFakeSecurity(), modules: frigateAPIModules(true), frigateUpstream: upstream}
			request := httptest.NewRequest("POST", "/api/v1/modules/catalog/frigate.module/install", strings.NewReader(test.body))
			response := httptest.NewRecorder()
			s.queueFrigateMutation(response, request, security.Actor{}, test.source, false)
			if response.Code != test.want {
				t.Fatalf("got %d: %s", response.Code, response.Body.String())
			}
			if test.want != 202 && len(jobs.jobs) != 0 {
				t.Fatal("unapproved image was queued")
			}
			if test.want == 202 {
				if len(jobs.jobs) != 1 || upstream.resolveCalls != 1 {
					t.Fatal("missing fresh verification or duplicate submission")
				}
				var manifest modules.Manifest
				if err := json.Unmarshal([]byte(jobs.jobs[0].Input["manifest_json"].(string)), &manifest); err != nil {
					t.Fatal(err)
				}
				if manifest.Runtime.Docker.Image != candidate.Image || manifest.ID != frigate.ModuleID {
					t.Fatal("queued an image other than the approved candidate")
				}
			}
		})
	}
}
func frigateGatewayServer(body string) *server {
	return &server{modules: frigateAPIModules(true), security: defaultFakeSecurity(), jobs: &uiJobService{},
		frigateEndpoint: func(context.Context) (string, error) { return "172.30.0.5", nil },
		frigateTransport: nvrRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
		}),
	}
}
func TestFrigateGatewayRequiresModuleManagementPermission(t *testing.T) {
	sec := defaultFakeSecurity()
	sec.actor.Permissions = []string{"modules.read", "security.self.read"}
	handler := uiAPI(sec, &uiJobService{}, uiModuleService{fakeModules: frigateAPIModules(true)})
	if result := uiRequest(handler, "GET", "/api/v1/frigate/api/config/raw", ""); result.Code != 403 {
		t.Fatalf("viewer read camera credentials: %d", result.Code)
	}
	sec.authErr = security.ErrUnauthorized
	if result := uiRequest(uiAPI(sec, &uiJobService{}, uiModuleService{}), "GET", "/api/v1/frigate/api/stats", ""); result.Code != 401 {
		t.Fatal(result.Code)
	}
}
func TestFrigateGatewayNeverForwardsCoreCredentials(t *testing.T) {
	s := frigateGatewayServer(`{"cameras":{}}`)
	calls := 0
	s.frigateTransport = nvrRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != "http://172.30.0.5:5000/api/stats" {
			t.Fatalf("unsafe upstream %s", r.URL)
		}
		for _, header := range []string{"Authorization", "Cookie", "X-CSRF-Token", "X-Forwarded-For", "Remote-User", "X-Frigate-User"} {
			if r.Header.Get(header) != "" {
				t.Fatalf("forwarded %s", header)
			}
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}, "Set-Cookie": []string{"injected=module"}}, Body: io.NopCloser(strings.NewReader(`{"cameras":{}}`))}, nil
	})
	r := httptest.NewRequest("GET", "/api/v1/frigate/api/stats?target=http://attacker.test", nil)
	for _, header := range []string{"Authorization", "Cookie", "X-CSRF-Token", "X-Forwarded-For", "Remote-User", "X-Frigate-User"} {
		r.Header.Set(header, "private-browser-value")
	}
	w := httptest.NewRecorder()
	s.frigateGateway(w, r, security.Actor{}, authBearer)
	if w.Code != 200 || calls != 1 || w.Header().Get("Set-Cookie") != "" {
		t.Fatalf("unsafe response %d %s", w.Code, w.Body.String())
	}
}
func TestFrigateGatewayRejectsUnreviewedRoutesAndHosts(t *testing.T) {
	for _, path := range []string{"restart", "users", "go2rtc/streams", "../config", "events/../clip.mp4", "snapshot/..", "http://attacker.test"} {
		s := frigateGatewayServer(`{}`)
		s.frigateEndpoint = func(context.Context) (string, error) {
			t.Fatal("unreviewed route resolved a container")
			return "", nil
		}
		w := httptest.NewRecorder()
		s.frigateGateway(w, httptest.NewRequest("GET", "/api/v1/frigate/api/"+path, nil), security.Actor{}, authBearer)
		if w.Code != 404 {
			t.Fatalf("route %s: %d", path, w.Code)
		}
	}
	for _, host := range []string{"127.0.0.1", "169.254.169.254", "8.8.8.8", "localhost", "172.30.0.5:8080"} {
		s := frigateGatewayServer(`{}`)
		s.frigateEndpoint = func(context.Context) (string, error) { return host, nil }
		s.frigateTransport = nvrRoundTripFunc(func(*http.Request) (*http.Response, error) { t.Fatal("contacted an untrusted host"); return nil, nil })
		w := httptest.NewRecorder()
		s.frigateGateway(w, httptest.NewRequest("GET", "/api/v1/frigate/api/stats", nil), security.Actor{}, authBearer)
		if w.Code != 502 {
			t.Fatalf("host %s: %d", host, w.Code)
		}
	}
}
func TestFrigateConfigWritesRequireCSRFAndIdleJobEngine(t *testing.T) {
	for _, test := range []struct {
		name   string
		source authSource
		jobs   []state.JobRecord
		want   int
	}{
		{name: "csrf", source: authCookie, want: 403},
		{name: "active", source: authBearer, jobs: []state.JobRecord{{Type: "module.upgrade", Status: "running"}}, want: 409},
		{name: "approved", source: authBearer, want: 200},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			s := frigateGatewayServer(`{"success":true}`)
			s.jobs = &uiJobService{jobs: test.jobs}
			s.frigateTransport = nvrRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				body, _ := io.ReadAll(r.Body)
				if string(body) != "mqtt:\n  enabled: false\n" || r.URL.Path != "/api/config/save" || r.URL.Query().Get("save_option") != "saveonly" {
					t.Fatal("configuration request was altered or restarted directly")
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"success":true}`))}, nil
			})
			w := httptest.NewRecorder()
			s.frigateGateway(w, httptest.NewRequest("POST", "/api/v1/frigate/api/config/save?save_option=restart", strings.NewReader("mqtt:\n  enabled: false\n")), security.Actor{}, test.source)
			if w.Code != test.want || (test.want != 200 && calls != 0) {
				t.Fatalf("got %d calls=%d: %s", w.Code, calls, w.Body.String())
			}
		})
	}
}
func TestFrigateGatewayBlocksInvalidAndRedirectResponses(t *testing.T) {
	for _, status := range []int{200, 307} {
		s := frigateGatewayServer(`<script>bad()</script>`)
		calls := 0
		s.frigateTransport = nvrRoundTripFunc(func(*http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"text/html"}, "Location": []string{"https://attacker.test"}}, Body: io.NopCloser(strings.NewReader(`<script>bad()</script>`))}, nil
		})
		w := httptest.NewRecorder()
		s.frigateGateway(w, httptest.NewRequest("GET", "/api/v1/frigate/api/stats", nil), security.Actor{}, authBearer)
		if w.Code != 502 || calls != 1 || strings.Contains(w.Body.String(), "<script>") {
			t.Fatal("unsafe response was relayed")
		}
	}
}
func TestFrigateClipPreservesByteRangesWithoutBrowserCredentials(t *testing.T) {
	s := frigateGatewayServer("")
	s.frigateTransport = nvrRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Range") != "bytes=1-3" || r.Header.Get("Cookie") != "" {
			t.Fatal("range or credential handling failed")
		}
		return &http.Response{StatusCode: 206, Header: http.Header{"Content-Type": []string{"video/mp4"}, "Content-Range": []string{"bytes 1-3/9"}, "Content-Length": []string{"3"}}, Body: io.NopCloser(strings.NewReader("mp4"))}, nil
	})
	request := httptest.NewRequest("GET", "/api/v1/frigate/api/events/123.456-abc/clip.mp4", nil)
	request.Header.Set("Range", "bytes=1-3")
	request.Header.Set("Cookie", "secret=value")
	response := httptest.NewRecorder()
	s.frigateGateway(response, request, security.Actor{}, authBearer)
	if response.Code != 206 || response.Header().Get("Content-Range") != "bytes 1-3/9" || response.Body.String() != "mp4" {
		t.Fatalf("invalid ranged clip %d", response.Code)
	}
}

func TestFrigateCatalogSourceIsExplicitAndAllowlisted(t *testing.T) {
	valid := moduleUpstreamSource{ID: frigate.ModuleID, Name: "Frigate NVR", Provider: "github-ghcr-official", Repository: frigate.Repository, ImageRepository: frigate.ImageRepository}
	for _, sources := range [][]moduleUpstreamSource{nil, {}, {{ID: "other.module", Provider: "arbitrary"}}} {
		enabled, err := frigateSourceEnabled(sources)
		if err != nil || enabled {
			t.Fatal("implicit Frigate source enabled", err)
		}
	}
	if enabled, err := frigateSourceEnabled([]moduleUpstreamSource{valid}); err != nil || !enabled {
		t.Fatal("official source disabled", err)
	}
	invalid := valid
	invalid.Repository = "attacker/frigate"
	for _, sources := range [][]moduleUpstreamSource{{invalid}, {valid, valid}} {
		if enabled, err := frigateSourceEnabled(sources); err == nil || enabled {
			t.Fatal("untrusted source enabled")
		}
	}
	upstream := &fakeFrigateUpstream{candidate: frigateAPICandidate()}
	s := &server{frigateUpstream: upstream}
	catalog := moduleCatalogDocument{Modules: []modules.Manifest{{ID: "nvr.module"}, {ID: frigate.ModuleID}}}
	absent := s.expandFrigateCatalog(context.Background(), catalog, false)
	if len(absent.Modules) != 1 || upstream.discoverCalls != 0 {
		t.Fatal("source-free catalog performed a network lookup or retained a static Frigate image")
	}
	catalog.Upstreams = []moduleUpstreamSource{valid}
	expanded := s.expandFrigateCatalog(context.Background(), catalog, false)
	if len(expanded.Modules) != 2 || expanded.FrigateUpstream == nil || expanded.Modules[1].Runtime.Docker.Image != upstream.candidate.Image {
		t.Fatal("official source not expanded to a verified per-host manifest")
	}
}
