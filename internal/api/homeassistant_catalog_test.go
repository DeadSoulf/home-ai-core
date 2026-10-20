package api

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/DeadSoulf/home-ai-core/internal/homeassistant"
	"github.com/DeadSoulf/home-ai-core/internal/modules"
)

type haCatalogTransport func(*http.Request) (*http.Response, error)

func (f haCatalogTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func haCatalogFixture() modules.Manifest {
	return modules.Manifest{SchemaVersion: 2, ID: homeassistant.ModuleID, Name: "Home Assistant", Version: "2026.10.1", Core: homeassistant.MinimumCore,
		Runtime:      modules.RuntimeSpec{Type: "docker", Docker: modules.DockerSpec{Image: homeassistant.Repository + "@sha256:" + strings.Repeat("a", 64)}, Health: modules.HealthSpec{Port: 8123, Path: "/"}},
		Capabilities: modules.Capabilities{Requires: []string{"host.docker"}}, Permissions: []string{"host.network", "host.privileged"}, Lifecycle: []string{"install", "upgrade", "remove"}}
}

func TestHomeAssistantCatalogOutagePreservesPinnedManifest(t *testing.T) {
	original := haCatalogFixture()
	catalog := moduleCatalogDocument{Modules: []modules.Manifest{original}}
	refreshHomeAssistantCandidate(context.Background(), &catalog, haCatalogTransport(func(*http.Request) (*http.Response, error) { return nil, errors.New("upstream unavailable") }))
	if catalog.HomeAssistantStatus != "upstream_unavailable" || catalog.Modules[0].Runtime.Docker.Image != original.Runtime.Docker.Image || catalog.Modules[0].Version != original.Version {
		t.Fatalf("fallback changed catalog: %#v", catalog)
	}
}

func TestHomeAssistantCatalogDoesNotContactUpstreamWithoutEntry(t *testing.T) {
	catalog := moduleCatalogDocument{}
	refreshHomeAssistantCandidate(context.Background(), &catalog, haCatalogTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("upstream contacted without explicit published entry")
		return nil, nil
	}))
}

func TestHomeAssistantCatalogAcceptsOnlyNewerVersionAndNeverRepinsSameVersion(t *testing.T) {
	for _, version := range []string{"2026.10.2", "2026.10.1", "2026.9.3"} {
		t.Run(version, func(t *testing.T) {
			original := haCatalogFixture()
			catalog := moduleCatalogDocument{Modules: []modules.Manifest{original}}
			transport := haCatalogTransport(func(r *http.Request) (*http.Response, error) {
				body := ""
				headers := http.Header{}
				switch {
				case r.URL.Host == "api.github.com":
					body = `{"tag_name":"` + version + `"}`
				case r.URL.Path == "/token":
					body = `{"token":"public-token"}`
				default:
					body = `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[{"digest":"sha256:` + strings.Repeat("b", 64) + `","platform":{"os":"linux","architecture":"amd64"}}]}`
					headers.Set("Docker-Content-Digest", fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(body))))
				}
				return &http.Response{StatusCode: 200, Header: headers, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})
			refreshHomeAssistantCandidate(context.Background(), &catalog, transport)
			if version == "2026.10.2" {
				if catalog.HomeAssistantStatus != "available" || catalog.Modules[0].Version != version || catalog.Modules[0].Runtime.Docker.Image == original.Runtime.Docker.Image {
					t.Fatal("new release not offered")
				}
				if err := modules.ValidateManifest(catalog.Modules[0]); err != nil {
					t.Fatal(err)
				}
			} else {
				if catalog.Modules[0].Runtime.Docker.Image != original.Runtime.Docker.Image || catalog.Modules[0].Version != original.Version {
					t.Fatal("existing version silently repinned/downgraded")
				}
			}
		})
	}
}
