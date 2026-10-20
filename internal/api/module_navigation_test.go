package api

import (
	"encoding/json"
	"github.com/DeadSoulf/home-ai-core/internal/modules"
	"github.com/DeadSoulf/home-ai-core/internal/security"
	"net/http/httptest"
	"testing"
)

func TestInstalledNavigationFallbackAndLifecycle(t *testing.T) {
	actor := security.Actor{Permissions: []string{"modules.read", "modules.manage"}}
	for _, status := range []string{"enabled", "disabled", "error", "registered"} {
		s := &server{modules: fakeModules{items: []modules.Registered{{Manifest: modules.Manifest{ID: "frigate.module", Name: "Frigate"}, Status: status}}}}
		w := httptest.NewRecorder()
		s.moduleNavigation(w, httptest.NewRequest("GET", "/api/v1/modules/navigation", nil), actor, authBearer)
		var result struct {
			Modules []moduleNavigationEntry `json:"modules"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if len(result.Modules) != 1 || result.Modules[0].Status != status || len(result.Modules[0].Items) != 1 || result.Modules[0].Items[0].Route != "/modules/frigate.module" {
			t.Fatalf("missing registry navigation for %s: %s", status, w.Body.String())
		}
	}
	s := &server{modules: fakeModules{}}
	w := httptest.NewRecorder()
	s.moduleNavigation(w, httptest.NewRequest("GET", "/api/v1/modules/navigation", nil), actor, authBearer)
	var result struct {
		Modules []moduleNavigationEntry `json:"modules"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	if len(result.Modules) != 0 {
		t.Fatal("removed/catalog-only modules appeared in sidebar")
	}
}
func TestInstalledNavigationDeduplicatesAndRejectsForeignRoutes(t *testing.T) {
	manifest := modules.Manifest{ID: "frigate.module", Name: "Frigate", UI: modules.UIContract{Navigation: []modules.NavigationItem{
		{ID: "one", Title: "Frigate", Route: "/modules/frigate.module"}, {ID: "two", Title: "Duplicate", Route: "/modules/frigate.module"},
		{ID: "foreign", Title: "Other", Route: "/modules/ai.agent"}, {ID: "traversal", Title: "Invalid", Route: "/modules/frigate.module/../ai.agent"},
	}}}
	items := installedModuleNavigation(manifest)
	if len(items) != 1 || items[0].Title != "Frigate" {
		t.Fatalf("duplicate or unsafe navigation: %#v", items)
	}
}
func TestInstalledNavigationPermissionFiltering(t *testing.T) {
	items := []modules.Registered{}
	for _, id := range []string{"frigate.module", "ai.agent", "nvr.module", "homeassistant.module", "reference.module"} {
		items = append(items, modules.Registered{Manifest: modules.Manifest{ID: id, Name: id}, Status: "enabled"})
	}
	for _, test := range []struct {
		permissions []string
		want        int
	}{
		{[]string{"security.self.read"}, 0}, {[]string{"modules.read"}, 3}, {[]string{"modules.manage"}, 2}, {[]string{"modules.read", "modules.manage"}, 5},
	} {
		s := &server{modules: fakeModules{items: items}}
		w := httptest.NewRecorder()
		s.moduleNavigation(w, httptest.NewRequest("GET", "/api/v1/modules/navigation", nil), security.Actor{Permissions: test.permissions}, authBearer)
		var result struct {
			Modules []moduleNavigationEntry `json:"modules"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if len(result.Modules) != test.want {
			t.Fatalf("permissions %v: %s", test.permissions, w.Body.String())
		}
	}
}
