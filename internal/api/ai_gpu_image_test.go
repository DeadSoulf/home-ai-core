package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DeadSoulf/home-ai-core/internal/modules"
)

func aiGPU030Module() fakeModules {
	return fakeModules{items: []modules.Registered{{
		Manifest: modules.Manifest{ID: "ai.agent", Name: "AI Agent", Version: "0.3.0"},
		Status:   "enabled",
	}}}
}

func TestAIGPUPrepareRequiresModuleManageAndCookieCSRF(t *testing.T) {
	sec := defaultFakeSecurity()
	sec.actor.Permissions = []string{"security.self.read", "modules.read"}
	jobs := &uiJobService{}
	app := uiAPI(sec, jobs, uiModuleService{fakeModules: aiGPU030Module()})
	if got := uiRequest(app, http.MethodGet, "/api/v1/ai/gpu/runtime", ""); got.Code != 403 {
		t.Fatalf("viewer can inspect GPU runtime: %d", got.Code)
	}
	if got := uiRequest(app, http.MethodPost, "/api/v1/ai/gpu/runtime/prepare", "{}"); got.Code != 403 {
		t.Fatalf("viewer can queue GPU download: %d", got.Code)
	}
	if len(jobs.jobs) != 0 {
		t.Fatal("unauthorized GPU download queued")
	}

	sec = defaultFakeSecurity()
	app = uiAPI(sec, jobs, uiModuleService{fakeModules: aiGPU030Module()})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/gpu/runtime/prepare", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "test"})
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)
	if w.Code != 403 || len(jobs.jobs) != 0 {
		t.Fatalf("CSRF bypass: %d", w.Code)
	}
	if got := uiRequest(app, http.MethodPost, "/api/v1/ai/gpu/runtime/prepare", "{}"); got.Code != 202 {
		t.Fatalf("GPU prepare was not queued: %d %s", got.Code, got.Body.String())
	}
	if len(jobs.jobs) != 1 || jobs.jobs[0].Type != "module.gpu.prepare" || jobs.jobs[0].Input["module_id"] != "ai.agent" {
		t.Fatalf("wrong GPU preparation job: %+v", jobs.jobs)
	}
	if got := uiRequest(app, http.MethodPost, "/api/v1/ai/gpu/runtime/prepare", "{}"); got.Code != 409 {
		t.Fatalf("double GPU preparation queued: %d", got.Code)
	}
}

func TestAIGPUPrepareRejectsLegacyAgentWithoutCreatingJob(t *testing.T) {
	jobs := &uiJobService{}
	app := uiAPI(defaultFakeSecurity(), jobs, uiModuleService{fakeModules: aiGPUEnabledModule()})
	if got := uiRequest(app, http.MethodPost, "/api/v1/ai/gpu/runtime/prepare", "{}"); got.Code != 409 {
		t.Fatalf("legacy agent allowed GPU package: %d", got.Code)
	}
	if len(jobs.jobs) != 0 {
		t.Fatal("wrong module version triggered download")
	}
}
