// Package frigate implements the narrowly scoped, official Frigate upstream.
// It never accepts a repository, registry, URL or tag suffix from a browser.
package frigate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	ModuleID          = "frigate.module"
	Repository        = "blakeblackshear/frigate"
	ImageRepository   = "ghcr.io/" + Repository
	GitHubAPI         = "https://api.github.com/repos/" + Repository
	RegistryAPI       = "https://ghcr.io/v2/" + Repository
	MaxDocumentBytes  = 8 << 20
	SupportedVersions = ">=0.18.2 <0.19.0"
)

var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var digestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

// Compatibility is an explicit integration policy, not a claim that all future
// releases have been tested. A new minor/major must be reviewed before enabling it.
func CompatibleVersion(value string) bool {
	v, err := parseVersion(value)
	return err == nil && v[0] == 0 && v[1] == 18 && v[2] >= 2
}
func ValidImage(image string) bool {
	return strings.HasPrefix(image, ImageRepository+"@") && digestPattern.MatchString(strings.TrimPrefix(image, ImageRepository+"@"))
}
func parseVersion(value string) ([3]uint64, error) {
	var out [3]uint64
	if !versionPattern.MatchString(value) {
		return out, errors.New("invalid stable Frigate version")
	}
	for i, part := range strings.Split(value, ".") {
		n, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return out, err
		}
		out[i] = n
	}
	return out, nil
}
func newer(a, b string) bool {
	av, ae := parseVersion(a)
	bv, be := parseVersion(b)
	if ae != nil || be != nil {
		return false
	}
	for i := range av {
		if av[i] != bv[i] {
			return av[i] > bv[i]
		}
	}
	return false
}

type Release struct {
	Tag         string `json:"tag_name"`
	Draft       bool   `json:"draft"`
	Prerelease  bool   `json:"prerelease"`
	PublishedAt string `json:"published_at"`
}

func (r Release) stableVersion() (string, bool) {
	if r.Draft || r.Prerelease || !strings.HasPrefix(r.Tag, "v") {
		return "", false
	}
	v := strings.TrimPrefix(r.Tag, "v")
	_, err := parseVersion(v)
	return v, err == nil
}

type Candidate struct {
	Version      string    `json:"version"`
	Architecture string    `json:"architecture"`
	Image        string    `json:"image"`
	Digest       string    `json:"digest"`
	IndexDigest  string    `json:"index_digest"`
	ReleaseURL   string    `json:"release_url"`
	CheckedAt    time.Time `json:"checked_at"`
}
type Discovery struct {
	LatestVersion  string     `json:"latest_version"`
	SupportedRange string     `json:"supported_range"`
	Candidate      *Candidate `json:"candidate,omitempty"`
	Warning        string     `json:"warning,omitempty"`
	CheckedAt      time.Time  `json:"checked_at"`
}

type Client struct{ http *http.Client }

func NewClient(transport http.RoundTripper) *Client {
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &Client{http: &http.Client{Transport: transport, Timeout: 25 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}
func (c *Client) get(ctx context.Context, address, accept, token string) ([]byte, http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", "HomeAiCore-Frigate-Integration")
	req.Header.Set("Accept", accept)
	req.Header.Set("Accept-Encoding", "identity")
	if strings.HasPrefix(address, GitHubAPI+"/") {
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, errors.New("official Frigate upstream connection failed")
	}
	// GHCR may redirect config blobs to its signed CDN. Only this precise host
	// is accepted; the registry bearer token MUST NOT be forwarded to that host.
	if resp.StatusCode == http.StatusTemporaryRedirect && strings.HasPrefix(address, RegistryAPI+"/blobs/") {
		location := resp.Header.Get("Location")
		resp.Body.Close()
		target, err := url.Parse(location)
		if err != nil || target.Scheme != "https" || target.Host != "pkg-containers.githubusercontent.com" || target.User != nil {
			return nil, nil, errors.New("untrusted OCI blob redirect")
		}
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, location, nil)
		if err != nil {
			return nil, nil, err
		}
		req.Header.Set("Accept-Encoding", "identity")
		resp, err = c.http.Do(req)
		if err != nil {
			return nil, nil, errors.New("official OCI blob download failed")
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("official Frigate upstream returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxDocumentBytes+1))
	if err != nil {
		return nil, nil, err
	}
	if len(body) > MaxDocumentBytes {
		return nil, nil, errors.New("upstream document exceeds size limit")
	}
	return body, resp.Header, nil
}

func (c *Client) Discover(ctx context.Context, arch string) (Discovery, error) {
	result := Discovery{SupportedRange: SupportedVersions, CheckedAt: time.Now().UTC()}
	if arch != "amd64" && arch != "arm64" {
		return result, errors.New("Frigate requires Linux amd64 or arm64")
	}
	var versions []string
	for page := 1; page <= 3; page++ {
		body, _, err := c.get(ctx, fmt.Sprintf("%s/releases?per_page=100&page=%d", GitHubAPI, page), "application/vnd.github+json", "")
		if err != nil {
			return result, err
		}
		var releases []Release
		if err := json.Unmarshal(body, &releases); err != nil {
			return result, fmt.Errorf("decode official releases: %w", err)
		}
		for _, release := range releases {
			if v, ok := release.stableVersion(); ok {
				versions = append(versions, v)
			}
		}
		if len(releases) < 100 {
			break
		}
	}
	sort.Slice(versions, func(i, j int) bool { return newer(versions[i], versions[j]) })
	if len(versions) == 0 {
		return result, errors.New("official repository has no stable releases")
	}
	result.LatestVersion = versions[0]
	if !CompatibleVersion(result.LatestVersion) {
		result.Warning = "Latest upstream release is outside the reviewed integration range: " + SupportedVersions
	}
	for _, version := range versions {
		if !CompatibleVersion(version) {
			continue
		}
		candidate, err := c.Resolve(ctx, version, arch)
		if err != nil {
			return result, err
		}
		result.Candidate = &candidate
		return result, nil
	}
	result.Warning = "No official release matches the reviewed integration range: " + SupportedVersions
	return result, nil
}

// Resolve re-reads the exact official release and verifies the raw OCI index,
// selected Linux image manifest and config blob. It pins the platform digest,
// not a mutable tag. HTTPS/registry identity is the trust root; a digest is NOT
// represented as a publisher signature or an attestation.
func (c *Client) Resolve(ctx context.Context, version, arch string) (Candidate, error) {
	var result Candidate
	if !CompatibleVersion(version) {
		return result, fmt.Errorf("Frigate %q is outside supported range %s", version, SupportedVersions)
	}
	tag := version
	switch arch {
	case "amd64":
	case "arm64":
		tag += "-standard-arm64"
	default:
		return result, errors.New("unsupported Frigate architecture")
	}
	body, _, err := c.get(ctx, GitHubAPI+"/releases/tags/v"+version, "application/vnd.github+json", "")
	if err != nil {
		return result, err
	}
	var release Release
	if err := json.Unmarshal(body, &release); err != nil {
		return result, err
	}
	if v, ok := release.stableVersion(); !ok || v != version {
		return result, errors.New("release is not the requested official stable version")
	}
	tokenBody, _, err := c.get(ctx, "https://ghcr.io/token?service=ghcr.io&scope=repository:blakeblackshear/frigate:pull", "application/json", "")
	if err != nil {
		return result, err
	}
	var auth struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(tokenBody, &auth); err != nil {
		return result, err
	}
	if auth.Token == "" {
		auth.Token = auth.AccessToken
	}
	if auth.Token == "" || len(auth.Token) > 16384 {
		return result, errors.New("invalid GHCR pull token")
	}
	raw, header, err := c.get(ctx, RegistryAPI+"/manifests/"+tag, manifestAccept, auth.Token)
	if err != nil {
		return result, err
	}
	rootDigest := sum(raw)
	if !digestPattern.MatchString(header.Get("Docker-Content-Digest")) || header.Get("Docker-Content-Digest") != rootDigest {
		return result, errors.New("GHCR manifest digest does not match its response bytes")
	}
	var doc manifest
	if err := json.Unmarshal(raw, &doc); err != nil {
		return result, err
	}
	if doc.SchemaVersion != 2 {
		return result, errors.New("unsupported OCI schema version")
	}
	selectedDigest := rootDigest
	if doc.MediaType == ociIndex || doc.MediaType == dockerIndex {
		var selected *descriptor
		for i := range doc.Manifests {
			d := &doc.Manifests[i]
			if d.Platform.OS == "linux" && d.Platform.Architecture == arch && (d.Platform.Variant == "" || (arch == "arm64" && d.Platform.Variant == "v8")) {
				if selected != nil {
					return result, errors.New("ambiguous platform manifests")
				}
				selected = d
			}
		}
		if selected == nil {
			return result, fmt.Errorf("official image has no linux/%s manifest", arch)
		}
		if selected.MediaType != ociManifest && selected.MediaType != dockerManifest {
			return result, errors.New("unsupported platform manifest type")
		}
		raw, err = c.verifiedObject(ctx, "manifests", *selected, auth.Token)
		if err != nil {
			return result, err
		}
		selectedDigest = selected.Digest
		if err := json.Unmarshal(raw, &doc); err != nil {
			return result, err
		}
	}
	if doc.SchemaVersion != 2 || (doc.MediaType != ociManifest && doc.MediaType != dockerManifest) {
		return result, errors.New("unsupported OCI image manifest")
	}
	config, err := c.verifiedObject(ctx, "blobs", doc.Config, auth.Token)
	if err != nil {
		return result, err
	}
	var platform struct {
		OS           string `json:"os"`
		Architecture string `json:"architecture"`
	}
	if err := json.Unmarshal(config, &platform); err != nil {
		return result, err
	}
	if platform.OS != "linux" || platform.Architecture != arch {
		return result, errors.New("image config architecture does not match the requested platform")
	}
	return Candidate{Version: version, Architecture: arch, Image: ImageRepository + "@" + selectedDigest,
		Digest: selectedDigest, IndexDigest: rootDigest, ReleaseURL: "https://github.com/" + Repository + "/releases/tag/v" + version,
		CheckedAt: time.Now().UTC()}, nil
}

const (
	ociIndex       = "application/vnd.oci.image.index.v1+json"
	dockerIndex    = "application/vnd.docker.distribution.manifest.list.v2+json"
	ociManifest    = "application/vnd.oci.image.manifest.v1+json"
	dockerManifest = "application/vnd.docker.distribution.manifest.v2+json"
	manifestAccept = ociIndex + ", " + dockerIndex + ", " + ociManifest + ", " + dockerManifest
)

type descriptor struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
	Platform  struct {
		OS           string `json:"os"`
		Architecture string `json:"architecture"`
		Variant      string `json:"variant"`
	} `json:"platform"`
}
type manifest struct {
	SchemaVersion int          `json:"schemaVersion"`
	MediaType     string       `json:"mediaType"`
	Manifests     []descriptor `json:"manifests"`
	Config        descriptor   `json:"config"`
}

func sum(body []byte) string { s := sha256.Sum256(body); return "sha256:" + hex.EncodeToString(s[:]) }
func (c *Client) verifiedObject(ctx context.Context, kind string, d descriptor, token string) ([]byte, error) {
	if !digestPattern.MatchString(d.Digest) || d.Size <= 0 || d.Size > MaxDocumentBytes {
		return nil, errors.New("invalid OCI descriptor")
	}
	body, header, err := c.get(ctx, RegistryAPI+"/"+kind+"/"+d.Digest, manifestAccept+", application/octet-stream", token)
	if err != nil {
		return nil, err
	}
	if int64(len(body)) != d.Size || sum(body) != d.Digest {
		return nil, errors.New("OCI object digest or size mismatch")
	}
	if advertised := header.Get("Docker-Content-Digest"); advertised != "" && advertised != d.Digest {
		return nil, errors.New("OCI object header digest mismatch")
	}
	return body, nil
}
