package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DeadSoulf/home-ai-core/internal/modules"
)

const testedPrivateNVRManifest = `{
  "schema_version": 2,
  "id": "nvr.module",
  "name": "NVR",
  "version": "1.1.6",
  "core": ">=0.1.200 <1.0.0",
  "runtime": {
    "type": "docker",
    "docker": {
      "image": "ghcr.io/deadsoulf/home-ai-core_dvr-sdk611@sha256:d61856e76052d6413eb2aed9bee72b38c077a0c7bf35445094d248c571d3961e"
    },
    "health": {"port": 8090, "path": "/api/nvr/status"}
  },
  "capabilities": {"requires": ["host.docker"], "provides": ["nvr.service"]},
  "lifecycle": ["install", "upgrade", "remove", "start", "stop", "restart"]
}`

func TestPrivateNVRCatalogIsPinnedAndAuthenticated(t *testing.T) {
	var seenAuthorization string
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuthorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(githubContentResponse{
			Content:  base64.StdEncoding.EncodeToString([]byte(testedPrivateNVRManifest)),
			Encoding: "base64",
		})
	}))
	defer service.Close()

	manifest, err := fetchPrivateNVRManifest(context.Background(), service.Client(), "private-auth-test", service.URL)
	if err != nil {
		t.Fatalf("fetch private NVR metadata: %v", err)
	}
	if seenAuthorization != "Bearer private-auth-test" {
		t.Fatal("private metadata request did not carry configured authentication")
	}
	if manifest.ID != "nvr.module" || manifest.Version != "1.1.6" {
		t.Fatalf("unexpected private metadata: %q %q", manifest.ID, manifest.Version)
	}

	stable := moduleCatalogDocument{Modules: []modules.Manifest{
		{ID: "nvr.module", Version: "1.1.4"},
		{ID: "ai.agent", Version: "0.3.2"},
	}}
	if status := mergePrivateNVRCandidate(&stable, manifest); status != "available" {
		t.Fatalf("private NVR candidate not selected: %q", status)
	}
	if stable.Modules[0].Version != "1.1.6" || stable.Modules[1].Version != "0.3.2" {
		t.Fatal("private candidate should replace only NVR on the opted-in host")
	}
}

func TestPrivateNVROverrideCannotDowngradeThePublicCatalog(t *testing.T) {
	var candidate modules.Manifest
	if err := json.Unmarshal([]byte(testedPrivateNVRManifest), &candidate); err != nil {
		t.Fatal(err)
	}
	stable := moduleCatalogDocument{Modules: []modules.Manifest{{ID: "nvr.module", Version: "1.1.10"}}}
	if state := mergePrivateNVRCandidate(&stable, candidate); state != "up_to_date" {
		t.Fatalf("unexpected private candidate downgrade: %q", state)
	}
	if stable.Modules[0].Version != "1.1.10" {
		t.Fatal("public stable candidate was unexpectedly downgraded")
	}
	for _, pair := range [][2]string{
		{"1.1.6", "1.1.4"}, {"1.1.10", "1.1.9"}, {"2.0.0", "1.99.99"},
	} {
		if !newerNVRSemver(pair[0], pair[1]) {
			t.Fatalf("%s must be newer than %s", pair[0], pair[1])
		}
	}
	if newerNVRSemver("1.1.6b", "1.1.4") || newerNVRSemver("1.1.6", "1.1.6") {
		t.Fatal("invalid or equal private versions must not replace public stable")
	}
}

func TestPrivateNVRMetadataFailsClosedOnForbiddenAndUntrustedImages(t *testing.T) {
	t.Run("private repository unavailable", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "private", http.StatusForbidden)
		}))
		defer srv.Close()
		if _, err := fetchPrivateNVRManifest(context.Background(), srv.Client(), "token", srv.URL); err == nil {
			t.Fatal("private metadata authorization failure must not permit an upgrade")
		}
	})
	t.Run("unexpected registry", func(t *testing.T) {
		manifest := strings.Replace(testedPrivateNVRManifest, "home-ai-core_dvr-sdk611@", "untrusted-image@", 1)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(githubContentResponse{
				Content: base64.StdEncoding.EncodeToString([]byte(manifest)),
				Encoding: "base64",
			})
		}))
		defer srv.Close()
		if _, err := fetchPrivateNVRManifest(context.Background(), srv.Client(), "token", srv.URL); err == nil {
			t.Fatal("private metadata must never permit arbitrary registries")
		}
	})
}
