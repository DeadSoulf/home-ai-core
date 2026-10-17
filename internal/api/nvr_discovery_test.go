package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DeadSoulf/home-ai-core/internal/nvrdiscovery"
	"github.com/DeadSoulf/home-ai-core/internal/security"
)

func testDiscoveryNetwork() nvrdiscovery.Network {
	return nvrdiscovery.Network{
		ID:        "eth0|192.168.42.0/24",
		Interface: "eth0", Address: "192.168.42.4", CIDR: "192.168.42.0/24", Hosts: 253,
	}
}

func TestNVRDiscoveryNetworksRequireManagerPermission(t *testing.T) {
	jobs := &uiJobService{}
	securityService := defaultFakeSecurity()
	securityService.actor.Permissions = []string{"modules.read"}
	handler := uiAPI(securityService, jobs, uiModuleService{fakeModules: nvrTestService(true)})
	if got := uiRequest(handler, "GET", "/api/v1/nvr/discovery/networks", ""); got.Code != http.StatusForbidden {
		t.Fatalf("read-only actor obtained local interfaces: %d", got.Code)
	}
	if got := uiRequest(handler, "POST", "/api/v1/nvr/discovery/search", `{"network_id":"eth0|192.168.42.0/24"}`); got.Code != http.StatusForbidden {
		t.Fatalf("read-only actor triggered active scan: %d", got.Code)
	}
	securityService.authErr = security.ErrUnauthorized
	handler = uiAPI(securityService, jobs, uiModuleService{fakeModules: nvrTestService(true)})
	if got := uiRequest(handler, "GET", "/api/v1/nvr/discovery/networks", ""); got.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated actor obtained interfaces: %d", got.Code)
	}
}

func TestNVRDiscoverySearchRejectsUnlistedNetworkAndRateLimits(t *testing.T) {
	allowed := testDiscoveryNetwork()
	actor := defaultFakeSecurity().actor
	calls := 0
	s := &server{
		modules:  nvrTestService(true),
		security: defaultFakeSecurity(),
		nvrNetworks: func() ([]nvrdiscovery.Network, error) {
			return []nvrdiscovery.Network{allowed}, nil
		},
		nvrScanner: func(ctx context.Context, n nvrdiscovery.Network) ([]nvrdiscovery.Device, error) {
			calls++
			if n.ID != allowed.ID {
				t.Fatalf("untrusted scope %v", n)
			}
			return []nvrdiscovery.Device{{
				Address: "192.168.42.20", Vendor: "Axis", Services: []string{"RTSP"},
				RTSPPort: 554, Confidence: "detected",
			}}, nil
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nvr/discovery/networks", nil)
	w := httptest.NewRecorder()
	s.nvrDiscoveryNetworks(w, req, actor, authBearer)
	if w.Code != 200 || !strings.Contains(w.Body.String(), allowed.CIDR) {
		t.Fatalf("networks %d %s", w.Code, w.Body.String())
	}
	for _, payload := range []string{
		`{"network_id":"8.8.8.0/24"}`, `{"network_id":"eth0|192.168.43.0/24"}`,
		`{"network_id":"eth0|192.168.42.0/24","unexpected":true}`,
		`{"network_id":"eth0|192.168.42.0/24"}{"another":1}`,
		`{}`,
	} {
		w = httptest.NewRecorder()
		s.nvrDiscover(w, httptest.NewRequest(http.MethodPost, "/api/v1/nvr/discovery/search", strings.NewReader(payload)), actor, authBearer)
		if w.Code != 400 || calls != 0 {
			t.Fatalf("invalid scope accepted %q: status %d, calls %d", payload, w.Code, calls)
		}
	}
	good := `{"network_id":"eth0|192.168.42.0/24"}`
	w = httptest.NewRecorder()
	s.nvrDiscover(w, httptest.NewRequest(http.MethodPost, "/api/v1/nvr/discovery/search", strings.NewReader(good)), actor, authBearer)
	if w.Code != 200 || calls != 1 {
		t.Fatalf("valid search failed %d %s", w.Code, w.Body.String())
	}
	var parsed struct {
		Devices      []nvrdiscovery.Device `json:"devices"`
		ScannedHosts int                   `json:"scanned_hosts"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Devices) != 1 || parsed.Devices[0].Vendor != "Axis" || parsed.ScannedHosts != 253 {
		t.Fatalf("unexpected result %+v", parsed)
	}
	w = httptest.NewRecorder()
	s.nvrDiscover(w, httptest.NewRequest(http.MethodPost, "/api/v1/nvr/discovery/search", strings.NewReader(good)), actor, authBearer)
	if w.Code != http.StatusTooManyRequests || calls != 1 {
		t.Fatalf("scan cooldown ignored %d %d", w.Code, calls)
	}
}

func TestNVRDiscoveryRejectsMissingCSRFAndDisabledModule(t *testing.T) {
	actor := defaultFakeSecurity().actor
	s := &server{modules: nvrTestService(true), nvrNetworks: func() ([]nvrdiscovery.Network, error) {
		return []nvrdiscovery.Network{testDiscoveryNetwork()}, nil
	}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/nvr/discovery/search",
		strings.NewReader(`{"network_id":"eth0|192.168.42.0/24"}`))
	w := httptest.NewRecorder()
	s.nvrDiscover(w, req, actor, authCookie)
	if w.Code != 403 {
		t.Fatalf("cookie mutation without CSRF accepted: %d", w.Code)
	}
	s.modules = nvrTestService(false)
	w = httptest.NewRecorder()
	s.nvrDiscoveryNetworks(w, httptest.NewRequest(http.MethodGet, "/api/v1/nvr/discovery/networks", nil), actor, authBearer)
	if w.Code != 503 {
		t.Fatalf("disabled NVR allowed scan network list: %d", w.Code)
	}
}
