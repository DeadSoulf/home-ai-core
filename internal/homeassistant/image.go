// Package homeassistant contains the fixed upstream trust policy shared by Core
// and its privileged helper. It never accepts a caller-selected registry URL.
package homeassistant

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const (
	ModuleID         = "homeassistant.module"
	Repository       = "ghcr.io/home-assistant/home-assistant"
	MinimumCore      = ">=0.1.209 <1.0.0"
	latestReleaseURL = "https://api.github.com/repos/home-assistant/core/releases/latest"
	registryTokenURL = "https://ghcr.io/token?service=ghcr.io&scope=repository:home-assistant/home-assistant:pull"
	manifestURL      = "https://ghcr.io/v2/home-assistant/home-assistant/manifests/"
)

var imagePattern = regexp.MustCompile(`^ghcr\.io/home-assistant/home-assistant@sha256:[a-f0-9]{64}$`)
var releasePattern = regexp.MustCompile(`^20[0-9]{2}\.(?:[1-9]|1[0-2])\.(?:0|[1-9][0-9]*)$`)

func ValidateImage(image string) error {
	if !imagePattern.MatchString(image) {
		return errors.New("Home Assistant requires the official ghcr.io/home-assistant/home-assistant image pinned by sha256")
	}
	return nil
}

type Release struct {
	Version       string   `json:"version"`
	Image         string   `json:"image"`
	Architectures []string `json:"architectures"`
}

// ResolveLatest discovers a stable upstream release and hashes the actual GHCR
// index bytes. This verifies registry content integrity, not a publisher signature.
// No HomeAiCore catalog token is sent to the third-party image repository.
func ResolveLatest(ctx context.Context, transport http.RoundTripper) (Release, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	client := &http.Client{Transport: transport, Timeout: 8 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("upstream metadata redirects are not allowed")
		}}
	get := func(url, accept, token string) ([]byte, http.Header, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, nil, err
		}
		req.Header.Set("User-Agent", "HomeAiCore-HomeAssistant")
		req.Header.Set("Accept", accept)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, nil, fmt.Errorf("read official Home Assistant metadata: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, nil, fmt.Errorf("official Home Assistant metadata returned HTTP %d", resp.StatusCode)
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
		if err != nil {
			return nil, nil, err
		}
		if len(data) > 4<<20 {
			return nil, nil, errors.New("Home Assistant metadata exceeds size limit")
		}
		return data, resp.Header, nil
	}
	data, _, err := get(latestReleaseURL, "application/vnd.github+json", "")
	if err != nil {
		return Release{}, err
	}
	var release struct {
		Tag        string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	if err := json.Unmarshal(data, &release); err != nil {
		return Release{}, err
	}
	if release.Draft || release.Prerelease || !releasePattern.MatchString(release.Tag) {
		return Release{}, errors.New("upstream release is not a stable Home Assistant version")
	}
	data, _, err = get(registryTokenURL, "application/json", "")
	if err != nil {
		return Release{}, err
	}
	var auth struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(data, &auth); err != nil {
		return Release{}, err
	}
	if auth.Token == "" {
		auth.Token = auth.AccessToken
	}
	if auth.Token == "" || len(auth.Token) > 16384 || strings.ContainsAny(auth.Token, "\r\n") {
		return Release{}, errors.New("invalid public GHCR pull token")
	}
	data, headers, err := get(manifestURL+release.Tag, "application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json", auth.Token)
	if err != nil {
		return Release{}, err
	}
	sum := sha256.Sum256(data)
	digest := "sha256:" + hex.EncodeToString(sum[:])
	if headers.Get("Docker-Content-Digest") != digest {
		return Release{}, errors.New("Home Assistant registry digest does not match downloaded index")
	}
	var index struct {
		SchemaVersion int    `json:"schemaVersion"`
		MediaType     string `json:"mediaType"`
		Manifests     []struct {
			Digest   string `json:"digest"`
			Platform struct {
				OS           string `json:"os"`
				Architecture string `json:"architecture"`
			} `json:"platform"`
		} `json:"manifests"`
	}
	if err := json.Unmarshal(data, &index); err != nil {
		return Release{}, err
	}
	if index.SchemaVersion != 2 || (index.MediaType != "application/vnd.oci.image.index.v1+json" && index.MediaType != "application/vnd.docker.distribution.manifest.list.v2+json") {
		return Release{}, errors.New("Home Assistant image is not a supported multi-platform index")
	}
	found := map[string]bool{}
	for _, item := range index.Manifests {
		if item.Platform.OS == "linux" && (item.Platform.Architecture == "amd64" || item.Platform.Architecture == "arm64") {
			if err := ValidateImage(Repository + "@" + item.Digest); err != nil {
				return Release{}, errors.New("invalid platform manifest digest")
			}
			found[item.Platform.Architecture] = true
		}
	}
	result := Release{Version: release.Tag, Image: Repository + "@" + digest}
	for _, arch := range []string{"amd64", "arm64"} {
		if found[arch] {
			result.Architectures = append(result.Architectures, arch)
		}
	}
	if len(result.Architectures) == 0 {
		return Release{}, errors.New("Home Assistant release has no supported Linux architecture")
	}
	return result, nil
}
