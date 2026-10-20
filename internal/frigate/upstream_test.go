package frigate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(body string, status int) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}
func jsonBytes(v any) []byte { b, _ := json.Marshal(v); return b }

type fixture struct {
	client               *Client
	root, image, config  []byte
	digest, configDigest string
	alter                func(*http.Request, *http.Response)
}

func newFixture(t *testing.T, arch string, index bool) *fixture {
	t.Helper()
	f := &fixture{}
	f.config = jsonBytes(map[string]any{"os": "linux", "architecture": arch})
	f.configDigest = sum(f.config)
	f.image = jsonBytes(manifest{SchemaVersion: 2, MediaType: ociManifest, Config: descriptor{Digest: f.configDigest, Size: int64(len(f.config))}})
	f.digest = sum(f.image)
	f.root = f.image
	if index {
		d := descriptor{MediaType: ociManifest, Digest: f.digest, Size: int64(len(f.image))}
		d.Platform.OS = "linux"
		d.Platform.Architecture = arch
		f.root = jsonBytes(manifest{SchemaVersion: 2, MediaType: ociIndex, Manifests: []descriptor{d}})
	}
	f.client = NewClient(transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "api.github.com" && r.Header.Get("Authorization") != "" {
			t.Fatal("registry token leaked to GitHub")
		}
		var res *http.Response
		switch {
		case r.URL.Host == "api.github.com" && strings.HasSuffix(r.URL.Path, "/releases"):
			res = response(`[{"tag_name":"v0.18.2"},{"tag_name":"v0.19.0-rc1","prerelease":true},{"tag_name":"v0.18.10","draft":true}]`, 200)
		case strings.Contains(r.URL.Path, "/releases/tags/"):
			res = response(`{"tag_name":"`+r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]+`"}`, 200)
		case r.URL.Path == "/token":
			res = response(`{"token":"fixture-token"}`, 200)
		case strings.HasSuffix(r.URL.Path, "/"+f.configDigest):
			res = response(string(f.config), 200)
		case strings.HasSuffix(r.URL.Path, "/"+f.digest):
			res = response(string(f.image), 200)
			res.Header.Set("Docker-Content-Digest", f.digest)
		case strings.Contains(r.URL.Path, "/manifests/"):
			res = response(string(f.root), 200)
			res.Header.Set("Docker-Content-Digest", sum(f.root))
		default:
			return nil, fmt.Errorf("unexpected URL %s", r.URL)
		}
		if f.alter != nil {
			f.alter(r, res)
		}
		return res, nil
	}))
	return f
}
func TestResolveVerifiedPlatformDigest(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		for _, index := range []bool{true, false} {
			t.Run(fmt.Sprint(arch, index), func(t *testing.T) {
				f := newFixture(t, arch, index)
				c, err := f.client.Resolve(context.Background(), "0.18.2", arch)
				if err != nil {
					t.Fatal(err)
				}
				if c.Digest != f.digest || c.Image != ImageRepository+"@"+f.digest || !ValidImage(c.Image) {
					t.Fatalf("bad candidate: %#v", c)
				}
			})
		}
	}
}
func TestResolveRejectsDigestCorruption(t *testing.T) {
	for _, part := range []string{"root", "child", "config", "header"} {
		t.Run(part, func(t *testing.T) {
			f := newFixture(t, "amd64", true)
			f.alter = func(r *http.Request, res *http.Response) {
				hit := (part == "root" && strings.HasSuffix(r.URL.Path, "/0.18.2")) || (part == "child" && strings.HasSuffix(r.URL.Path, "/"+f.digest)) || (part == "config" && strings.HasSuffix(r.URL.Path, "/"+f.configDigest))
				if hit {
					res.Body = io.NopCloser(strings.NewReader(`{"tampered":true}`))
				}
				if part == "header" && strings.HasSuffix(r.URL.Path, "/0.18.2") {
					res.Header.Set("Docker-Content-Digest", "sha256:"+strings.Repeat("0", 64))
				}
			}
			if _, err := f.client.Resolve(context.Background(), "0.18.2", "amd64"); err == nil {
				t.Fatal("accepted corrupted image")
			}
		})
	}
}
func TestResolveRejectsWrongPlatform(t *testing.T) {
	for _, index := range []bool{false, true} {
		f := newFixture(t, "arm64", index)
		if _, err := f.client.Resolve(context.Background(), "0.18.2", "amd64"); err == nil {
			t.Fatal("accepted wrong architecture")
		}
	}
}
func TestVersionPolicy(t *testing.T) {
	for _, v := range []string{"0.18.2", "0.18.10"} {
		if !CompatibleVersion(v) {
			t.Fatal(v)
		}
	}
	for _, v := range []string{"0.18.1", "0.19.0", "1.0.0", "v0.18.2", "0.18.02", "0.18.3-rc1", "../latest", "stable", "0.18.999999999999999999999999"} {
		if CompatibleVersion(v) {
			t.Fatal(v)
		}
	}
	for _, image := range []string{ImageRepository + ":stable", "ghcr.io/attacker/frigate@sha256:" + strings.Repeat("a", 64), ImageRepository + "@sha256:" + strings.Repeat("A", 64)} {
		if ValidImage(image) {
			t.Fatal(image)
		}
	}
}
func TestStableReleaseDiscoveryAndCompatibility(t *testing.T) {
	f := newFixture(t, "amd64", true)
	f.alter = func(r *http.Request, res *http.Response) {
		if strings.HasSuffix(r.URL.Path, "/releases") {
			res.Body = io.NopCloser(strings.NewReader(`[{"tag_name":"v0.18.2"},{"tag_name":"v0.18.10"},{"tag_name":"v0.19.0"},{"tag_name":"v9.0.0","prerelease":true}]`))
		}
	}
	d, err := f.client.Discover(context.Background(), "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if d.LatestVersion != "0.19.0" || d.Candidate == nil || d.Candidate.Version != "0.18.10" || d.Warning == "" {
		t.Fatalf("%#v", d)
	}
}
func TestRejectDraftOrMovedRelease(t *testing.T) {
	for _, body := range []string{`{"tag_name":"v0.18.2","draft":true}`, `{"tag_name":"v0.18.2","prerelease":true}`, `{"tag_name":"v0.18.3"}`} {
		f := newFixture(t, "amd64", true)
		f.alter = func(r *http.Request, res *http.Response) {
			if strings.Contains(r.URL.Path, "/releases/tags/") {
				res.Body = io.NopCloser(strings.NewReader(body))
			}
		}
		if _, err := f.client.Resolve(context.Background(), "0.18.2", "amd64"); err == nil {
			t.Fatal("accepted nonstable or moved release")
		}
	}
}
func TestRejectRateLimitAndOversizedDocument(t *testing.T) {
	for _, mode := range []string{"rate", "size"} {
		f := newFixture(t, "amd64", true)
		f.alter = func(r *http.Request, res *http.Response) {
			if strings.HasSuffix(r.URL.Path, "/releases") {
				if mode == "rate" {
					res.StatusCode = 429
				} else {
					res.Body = io.NopCloser(strings.NewReader(strings.Repeat("x", MaxDocumentBytes+1)))
				}
			}
		}
		if _, err := f.client.Discover(context.Background(), "amd64"); err == nil {
			t.Fatal(mode)
		}
	}
}
func TestCDNRedirectNeverForwardsToken(t *testing.T) {
	f := newFixture(t, "amd64", true)
	base := f.client.http.Transport
	f.client = NewClient(transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "pkg-containers.githubusercontent.com" {
			if r.Header.Get("Authorization") != "" {
				t.Fatal("token leak")
			}
			return response(string(f.config), 200), nil
		}
		res, err := base.RoundTrip(r)
		if err == nil && strings.HasSuffix(r.URL.Path, "/"+f.configDigest) {
			res.StatusCode = 307
			res.Header.Set("Location", "https://pkg-containers.githubusercontent.com/blob?signature=fixture")
		}
		return res, err
	}))
	if _, err := f.client.Resolve(context.Background(), "0.18.2", "amd64"); err != nil {
		t.Fatal(err)
	}
}
func TestRejectUntrustedRedirect(t *testing.T) {
	for _, location := range []string{"http://pkg-containers.githubusercontent.com/blob", "https://attacker.test/blob", "https://pkg-containers.githubusercontent.com:443/blob", "https://user@pkg-containers.githubusercontent.com/blob"} {
		f := newFixture(t, "amd64", true)
		f.alter = func(r *http.Request, res *http.Response) {
			if strings.HasSuffix(r.URL.Path, "/"+f.configDigest) {
				res.StatusCode = 307
				res.Header.Set("Location", location)
			}
		}
		if _, err := f.client.Resolve(context.Background(), "0.18.2", "amd64"); err == nil {
			t.Fatal(location)
		}
	}
}
func TestOfficialUpstreamLive(t *testing.T) {
	if os.Getenv("FRIGATE_LIVE_TEST") != "1" {
		t.Skip("explicit network integration test")
	}
	for _, arch := range []string{"amd64", "arm64"} {
		c, err := NewClient(nil).Resolve(context.Background(), "0.18.2", arch)
		if err != nil {
			t.Fatal(arch, err)
		}
		t.Logf("verified %s %s %s", c.Version, arch, c.Image)
	}
}
