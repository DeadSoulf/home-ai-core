package api

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/state"
	"github.com/coder/websocket"
)

type frigateUISecurityFixture struct {
	fakeSecurity
	revoked atomic.Bool
}

func (f *frigateUISecurityFixture) Authenticate(ctx context.Context, token string) (security.Actor, error) {
	if token != "fixture-ui-session" || f.revoked.Load() {
		return security.Actor{}, security.ErrUnauthorized
	}
	return f.fakeSecurity.Authenticate(ctx, token)
}
func fullUIFixture() *server {
	return &server{security: &frigateUISecurityFixture{fakeSecurity: defaultFakeSecurity()}, modules: frigateAPIModules(true), jobs: &uiJobService{},
		frigateUI:       &frigateUIState{port: "8971", corePort: "82", ready: true, context: context.Background(), recheck: 20 * time.Millisecond},
		frigateEndpoint: func(context.Context) (string, error) { return "172.30.0.5", nil },
	}
}
func fullUIRequest(method, path string) *http.Request {
	r := httptest.NewRequest(method, "https://home-ai.test:8971"+path, strings.NewReader(`{"fixture":true}`))
	r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "fixture-ui-session"})
	if method != "GET" && method != "HEAD" {
		r.Header.Set("Origin", "https://home-ai.test:8971")
	}
	return r
}
func fullUIResponse(body, contentType string) *http.Response {
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(body))}
}
func TestFullFrigateUIForwardsOriginalApplicationAndEntireAPI(t *testing.T) {
	for _, path := range []string{"/", "/review", "/explore", "/export", "/settings", "/assets/frigate.js", "/locales/ru/common.json", "/api/config/schema.json", "/api/review?limit=25&after=1", "/api/logs/frigate", "/api/go2rtc/streams", "/vod/camera/index.m3u8", "/exports/clip.mp4", "/recordings/fixture.mp4"} {
		s := fullUIFixture()
		s.frigateTransport = nvrRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Host != "172.30.0.5:5000" || r.URL.RequestURI() != path {
				t.Fatalf("upstream route altered: %s", r.URL)
			}
			return fullUIResponse("original-upstream-response", "application/octet-stream"), nil
		})
		w := httptest.NewRecorder()
		s.serveFrigateUI(w, fullUIRequest("GET", path))
		if w.Code != 200 || w.Body.String() != "original-upstream-response" {
			t.Fatalf("original route %s was filtered: %d %s", path, w.Code, w.Body.String())
		}
	}
}
func TestFullFrigateUICoreCredentialsNeverCrossBoundary(t *testing.T) {
	s := fullUIFixture()
	calls := 0
	s.frigateTransport = nvrRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		for _, name := range []string{"Cookie", "Authorization", "Remote-User", "X-Forwarded-For", "X-Proxy-Secret", "X-Ingress-Path", "X-Custom-Secret"} {
			if r.Header.Get(name) != "" {
				t.Fatalf("leaked header %s", name)
			}
		}
		if r.Header.Get("X-CSRF-Token") == "core-csrf-secret" {
			t.Fatal("leaked Core CSRF token")
		}
		res := fullUIResponse("<html>official Frigate</html>", "text/html")
		res.Header.Set("Set-Cookie", "home_ai_session=attacker; Path=/")
		res.Header.Set("Access-Control-Allow-Origin", "*")
		res.Header.Set("Clear-Site-Data", `"cookies"`)
		res.Header.Set("X-Frame-Options", "SAMEORIGIN")
		return res, nil
	})
	r := fullUIRequest("GET", "/")
	for _, name := range []string{"Remote-User", "X-Forwarded-For", "X-Proxy-Secret", "X-Ingress-Path", "X-Custom-Secret"} {
		r.Header.Set(name, "must-not-forward")
	}
	r.Header.Set("Authorization", "Bearer fixture-ui-session")
	r.Header.Set("X-CSRF-Token", "core-csrf-secret")
	w := httptest.NewRecorder()
	s.serveFrigateUI(w, r)
	if w.Code != 200 || calls != 1 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, name := range []string{"Set-Cookie", "Access-Control-Allow-Origin", "Clear-Site-Data", "X-Frame-Options"} {
		if w.Header().Get(name) != "" {
			t.Fatalf("unsafe upstream response header %s", name)
		}
	}
	if w.Header().Get("Content-Security-Policy") != "frame-ancestors https://home-ai.test:82" {
		t.Fatal("unrestricted framing")
	}
	if w.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("sensitive data may be cached")
	}
}
func TestFullFrigateUIRequiresManagerSessionForEveryResource(t *testing.T) {
	for _, kind := range []string{"missing", "revoked", "reader", "stopped", "wrong-host-port", "cross-site"} {
		s := fullUIFixture()
		r := fullUIRequest("GET", "/exports/private.mp4")
		want := 403
		switch kind {
		case "missing":
			r.Header.Del("Cookie")
			want = 401
		case "revoked":
			s.security.(*frigateUISecurityFixture).revoked.Store(true)
			want = 401
		case "reader":
			s.security.(*frigateUISecurityFixture).actor.Permissions = []string{"modules.read"}
		case "stopped":
			s.modules = frigateAPIModules(false)
			want = 503
		case "wrong-host-port":
			r.Host = "home-ai.test:82"
			want = 400
		case "cross-site":
			r.Header.Set("Sec-Fetch-Site", "cross-site")
		}
		s.frigateTransport = nvrRoundTripFunc(func(*http.Request) (*http.Response, error) {
			t.Fatal("unauthorized request reached container")
			return nil, nil
		})
		w := httptest.NewRecorder()
		s.serveFrigateUI(w, r)
		if w.Code != want {
			t.Fatalf("%s got %d: %s", kind, w.Code, w.Body.String())
		}
	}
}
func TestFullFrigateUIWritesAndWebSocketsRejectWrongOrigin(t *testing.T) {
	for _, origin := range []string{"", "null", "https://attacker.test", "https://home-ai.test:82", "http://home-ai.test:8971"} {
		for _, method := range []string{"POST", "PUT", "PATCH", "DELETE", "WS"} {
			s := fullUIFixture()
			r := fullUIRequest("POST", "/api/config/set")
			if method == "WS" {
				r.Method = "GET"
				r.URL.Path = "/ws"
				r.Header.Set("Upgrade", "websocket")
			} else {
				r.Method = method
			}
			r.Header.Set("Origin", origin)
			s.frigateTransport = nvrRoundTripFunc(func(*http.Request) (*http.Response, error) {
				t.Fatal("cross-origin write reached upstream")
				return nil, nil
			})
			w := httptest.NewRecorder()
			s.serveFrigateUI(w, r)
			if w.Code != 403 {
				t.Fatalf("%s origin %q got %d", method, origin, w.Code)
			}
		}
	}
}
func TestFullFrigateUIForwardsNativeConfigurationAndHonorsUpdateLock(t *testing.T) {
	for _, busy := range []bool{false, true} {
		s := fullUIFixture()
		if busy {
			s.jobs = &uiJobService{jobs: []state.JobRecord{{Type: "module.upgrade", Status: "running"}}}
		}
		calls := 0
		s.frigateTransport = nvrRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			body, _ := io.ReadAll(r.Body)
			if r.URL.Path != "/api/config/set" || string(body) != `{"fixture":true}` || r.Header.Get("X-CSRF-TOKEN") == "" {
				t.Fatal("native configuration body/header lost")
			}
			return fullUIResponse(`{"success":true}`, "application/json"), nil
		})
		w := httptest.NewRecorder()
		s.serveFrigateUI(w, fullUIRequest("POST", "/api/config/set"))
		if busy {
			if w.Code != 409 || calls != 0 {
				t.Fatal("mutated during pending upgrade")
			}
		} else if w.Code != 200 || calls != 1 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}
func TestFullFrigateUIPreservesRangesAndDoesNotFollowRedirects(t *testing.T) {
	s := fullUIFixture()
	s.frigateTransport = nvrRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Range") != "bytes=1-3" {
			t.Fatal("range lost")
		}
		res := fullUIResponse("mp4", "video/mp4")
		res.StatusCode = 206
		res.Header.Set("Content-Range", "bytes 1-3/100")
		return res, nil
	})
	r := fullUIRequest("GET", "/exports/test.mp4")
	r.Header.Set("Range", "bytes=1-3")
	w := httptest.NewRecorder()
	s.serveFrigateUI(w, r)
	if w.Code != 206 || w.Header().Get("Content-Range") != "bytes 1-3/100" || w.Body.String() != "mp4" {
		t.Fatal("range response broken")
	}
	s.frigateTransport = nvrRoundTripFunc(func(*http.Request) (*http.Response, error) {
		res := fullUIResponse("", "")
		res.StatusCode = 302
		res.Header.Set("Location", "https://attacker.test/")
		return res, nil
	})
	w = httptest.NewRecorder()
	s.serveFrigateUI(w, fullUIRequest("GET", "/"))
	if w.Code != 502 {
		t.Fatal("accepted external redirect")
	}
}
func TestFullFrigateUIRealWebSocketAndRevocation(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			t.Error("credentials leaked to WS")
		}
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.CloseNow()
		for {
			typ, data, err := conn.Read(r.Context())
			if err != nil {
				return
			}
			if err := conn.Write(r.Context(), typ, data); err != nil {
				return
			}
		}
	}))
	defer upstream.Close()
	upstreamURL, _ := url.Parse(upstream.URL)
	s := fullUIFixture()
	s.frigateTransport = &http.Transport{Proxy: nil, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", upstreamURL.Host)
	}}
	proxy := httptest.NewUnstartedServer(http.HandlerFunc(s.serveFrigateUI))
	_, s.frigateUI.port, _ = net.SplitHostPort(proxy.Listener.Addr().String())
	s.frigateUI.corePort = "82"
	proxy.Start()
	defer proxy.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	headers := http.Header{"Cookie": []string{sessionCookieName + "=fixture-ui-session"}, "Origin": []string{proxy.URL}}
	conn, _, err := websocket.Dial(ctx, proxy.URL+"/ws", &websocket.DialOptions{HTTPHeader: headers})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	if err := conn.Write(ctx, websocket.MessageText, []byte("native-ws-roundtrip")); err != nil {
		t.Fatal(err)
	}
	_, data, err := conn.Read(ctx)
	if err != nil || string(data) != "native-ws-roundtrip" {
		t.Fatal("WS relay failed", err)
	}
	s.security.(*frigateUISecurityFixture).revoked.Store(true)
	revokeCtx, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if _, _, err := conn.Read(revokeCtx); err == nil || revokeCtx.Err() != nil {
		t.Fatal("revoked session kept its WebSocket open", err)
	}
}
func TestFullFrigateUIOriginValidation(t *testing.T) {
	for _, host := range []string{"user@host:82", "host:82/path", "host:82?x=y", "host:82#x", "host:82\\evil", "host%0aevil:82"} {
		r := fullUIRequest("GET", "/")
		r.Host = host
		if _, err := frigateOrigin(r, "8971"); err == nil {
			t.Fatal(host)
		}
	}
	for _, host := range []string{"192.168.1.5:82", "localhost:82"} {
		r := httptest.NewRequest("GET", "http://"+host, nil)
		if _, err := frigateOrigin(r, "8971"); err == nil {
			t.Fatal("plaintext nonliteral-loopback accepted", host)
		}
	}
	r := httptest.NewRequest("GET", "https://[2001:db8::2]:82/", nil)
	got, err := frigateOrigin(r, "8971")
	if err != nil || got != "https://[2001:db8::2]:8971" {
		t.Fatal(got, err)
	}
}
