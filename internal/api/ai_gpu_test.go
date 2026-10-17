package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/DeadSoulf/home-ai-core/internal/modules"
	"github.com/DeadSoulf/home-ai-core/internal/security"
)

func aiGPUEnabledModule() fakeModules {
	return fakeModules{items: []modules.Registered{{
		Manifest: modules.Manifest{ID: "ai.agent", Version: "0.2.0"},
		Status: "enabled",
	}}}
}

func TestAIGPUCannotBeAssignedByViewer(t *testing.T) {
	sec := defaultFakeSecurity()
	sec.actor.Permissions = []string{"security.self.read", "modules.read"}
	router := uiAPI(sec, &uiJobService{}, uiModuleService{fakeModules: aiGPUEnabledModule()})
	if res := uiRequest(router, "GET", "/api/v1/ai/gpu/assignment", ""); res.Code != 403 {
		t.Fatalf("GPU inventory disclosed to viewer: %d", res.Code)
	}
	if res := uiRequest(router, "POST", "/api/v1/ai/gpu/assignment",
		`{"pci_addresses":["0000:01:00.0"],"confirmation":"ASSIGN_AI_GPU"}`); res.Code != 403 {
		t.Fatalf("GPU control granted to viewer: %d", res.Code)
	}
}

func TestAIGPUControlRequiresEnabledNewModuleAndCookieCSRF(t *testing.T) {
	s := aiTestServer(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ai/gpu/assignment", nil)
	w := httptest.NewRecorder()
	s.aiGPUAssignment(w, req, security.Actor{}, authBearer)
	if w.Code != http.StatusConflict {
		t.Fatalf("old AI image allowed GPU allocation: %d", w.Code)
	}
	s.modules = aiGPUEnabledModule()
	w = httptest.NewRecorder()
	s.aiGPUApply(w, httptest.NewRequest(http.MethodPost, "/api/v1/ai/gpu/assignment",
		strings.NewReader(`{"pci_addresses":[],"confirmation":"ASSIGN_AI_GPU"}`)), security.Actor{}, authCookie)
	if w.Code != http.StatusForbidden {
		t.Fatalf("GPU setting ignored CSRF: %d", w.Code)
	}
}

func TestAIGPUApplyRequiresExplicitConfirmationAndRejectsDuplicates(t *testing.T) {
	s := aiTestServer(t, nil)
	s.modules = aiGPUEnabledModule()
	calls := 0
	s.aiGPUApplyFunc = func(_ context.Context, values []string) ([]string, error) {
		calls++
		return values, nil
	}
	for _, request := range []string{
		`{"pci_addresses":["0000:01:00.0"],"confirmation":"wrong"}`,
		`{"pci_addresses":["../../etc/passwd"],"confirmation":"ASSIGN_AI_GPU"}`,
		`{"pci_addresses":["0000:01:00.0","0000:01:00.0"],"confirmation":"ASSIGN_AI_GPU"}`,
		`{"pci_addresses":null,"confirmation":"ASSIGN_AI_GPU"}`,
		`{"pci_addresses":[],"confirmation":"ASSIGN_AI_GPU","extra":"secret"}`,
	} {
		w := httptest.NewRecorder()
		s.aiGPUApply(w, httptest.NewRequest(http.MethodPost, "/api/v1/ai/gpu/assignment",
			strings.NewReader(request)), security.Actor{}, authBearer)
		if w.Code != http.StatusBadRequest {
			t.Errorf("malformed selection accepted: %d %s", w.Code, w.Body.String())
		}
	}
	if calls != 0 {
		t.Fatalf("unsafe input caused %d GPU mutations", calls)
	}
}

func TestAIGPUCanApplyTwoCardsAndReturnToCPU(t *testing.T) {
	s := aiTestServer(t, nil)
	s.modules = aiGPUEnabledModule()
	assigned := []string{}
	s.aiGPUStatus = func(context.Context) ([]string, error) { return assigned, nil }
	s.aiGPUApplyFunc = func(_ context.Context, pci []string) ([]string, error) {
		assigned = append([]string(nil), pci...)
		return assigned, nil
	}
	const path = "/api/v1/ai/gpu/assignment"
	input := `{"pci_addresses":["0000:01:00.0","0000:02:00.0"],"confirmation":"ASSIGN_AI_GPU"}`
	w := httptest.NewRecorder()
	s.aiGPUApply(w, httptest.NewRequest(http.MethodPost, path, strings.NewReader(input)),
		security.Actor{}, authBearer)
	if w.Code != http.StatusOK {
		t.Fatalf("two GPU switch returned: %d %s", w.Code, w.Body.String())
	}
	if !reflect.DeepEqual(assigned, []string{"0000:01:00.0", "0000:02:00.0"}) {
		t.Fatalf("wrong GPU set: %v", assigned)
	}
	w = httptest.NewRecorder()
	s.aiGPUAssignment(w, httptest.NewRequest(http.MethodGet, path, nil),
		security.Actor{}, authBearer)
	var status struct {
		Assigned []string `json:"assigned_gpu_pci"`
		Mode string `json:"mode"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Mode != "nvidia" || len(status.Assigned) != 2 {
		t.Fatalf("GPU status mismatch: %+v", status)
	}
	w = httptest.NewRecorder()
	s.aiGPUApply(w, httptest.NewRequest(http.MethodPost, path,
		strings.NewReader(`{"pci_addresses":[],"confirmation":"ASSIGN_AI_GPU"}`)),
		security.Actor{}, authBearer)
	if w.Code != http.StatusOK || len(assigned) != 0 {
		t.Fatalf("CPU rollback failed: %d %+v", w.Code, assigned)
	}
}
