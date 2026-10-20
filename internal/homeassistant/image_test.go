package homeassistant

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

type fakeTransport func(*http.Request) (*http.Response, error)

func (f fakeTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestOfficialImageAllowlist(t *testing.T) {
	valid := Repository + "@sha256:" + strings.Repeat("a", 64)
	if err := ValidateImage(valid); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{Repository + ":stable", valid + " ", strings.Replace(valid, "home-assistant/home-assistant", "attacker/home-assistant", 1), strings.Replace(valid, "ghcr.io/", "ghcr.io.evil/", 1), strings.Replace(valid, "sha256:", "sha512:", 1), Repository + "@sha256:" + strings.Repeat("A", 64), "--privileged", "", valid + "/../other"} {
		if ValidateImage(bad) == nil {
			t.Fatalf("untrusted image accepted: %q", bad)
		}
	}
}

func TestResolveLatestValidatesReleaseIndexAndPlatforms(t *testing.T) {
	index := `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[{"digest":"sha256:` + strings.Repeat("a", 64) + `","platform":{"os":"linux","architecture":"amd64"}},{"digest":"sha256:` + strings.Repeat("b", 64) + `","platform":{"os":"linux","architecture":"arm64"}},{"digest":"sha256:` + strings.Repeat("c", 64) + `","platform":{"os":"unknown","architecture":"unknown"}}]}`
	for _, test := range []struct {
		name, release, index, digest string
		want                         bool
	}{
		{"valid", `{"tag_name":"2026.10.2"}`, index, "", true},
		{"draft", `{"tag_name":"2026.10.2","draft":true}`, index, "", false},
		{"prerelease", `{"tag_name":"2026.10.2","prerelease":true}`, index, "", false},
		{"path injection", `{"tag_name":"../../other"}`, index, "", false},
		{"beta tag", `{"tag_name":"2026.10.0b1"}`, index, "", false},
		{"wrong digest", `{"tag_name":"2026.10.2"}`, index, "sha256:" + strings.Repeat("0", 64), false},
		{"no linux platform", `{"tag_name":"2026.10.2"}`, strings.ReplaceAll(index, `"linux"`, `"windows"`), "", false},
		{"invalid descriptor", `{"tag_name":"2026.10.2"}`, strings.ReplaceAll(index, "sha256:", "sha512:"), "", false},
		{"single platform manifest", `{"tag_name":"2026.10.2"}`, strings.Replace(index, "application/vnd.oci.image.index.v1+json", "application/vnd.oci.image.manifest.v1+json", 1), "", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			transport := fakeTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				body := ""
				headers := http.Header{}
				switch r.URL.String() {
				case latestReleaseURL:
					body = test.release
				case registryTokenURL:
					body = `{"token":"public-pull-token"}`
				case manifestURL + "2026.10.2":
					if r.Header.Get("Authorization") != "Bearer public-pull-token" {
						t.Fatal("missing scoped anonymous registry token")
					}
					body = test.index
					digest := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(body)))
					if test.digest != "" {
						digest = test.digest
					}
					headers.Set("Docker-Content-Digest", digest)
				default:
					t.Fatalf("unexpected metadata target: %s", r.URL)
				}
				if r.URL.String() != manifestURL+"2026.10.2" && r.Header.Get("Authorization") != "" {
					t.Fatal("credentials leaked to metadata request")
				}
				return &http.Response{StatusCode: 200, Header: headers, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})
			result, err := ResolveLatest(context.Background(), transport)
			if (err == nil) != test.want {
				t.Fatalf("release=%#v err=%v", result, err)
			}
			if test.want && (calls != 3 || result.Version != "2026.10.2" || len(result.Architectures) != 2 || ValidateImage(result.Image) != nil) {
				t.Fatalf("invalid resolved release: %#v, calls=%d", result, calls)
			}
		})
	}
}

func TestResolveLatestRefusesRedirectsAndNonOK(t *testing.T) {
	for _, status := range []int{302, 401, 404, 429, 500} {
		calls := 0
		_, err := ResolveLatest(context.Background(), fakeTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: status, Header: http.Header{"Location": []string{"https://untrusted.invalid/"}}, Body: io.NopCloser(strings.NewReader("secret must not appear in error")), Request: r}, nil
		}))
		if err == nil || calls != 1 || strings.Contains(err.Error(), "secret must") {
			t.Fatalf("status=%d calls=%d err=%v", status, calls, err)
		}
	}
}
