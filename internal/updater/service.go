package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	releasesURL      = "https://api.github.com/repos/DeadSoulf/home-ai-core/releases?per_page=20"
	maxReleaseBytes  = 1 << 20
	defaultUserAgent = "Home-AI-Core"
)

var ErrNoUpdate = errors.New("no update available")

type ReleaseStatus struct {
	CurrentVersion   string     `json:"current_version"`
	AvailableVersion string     `json:"available_version,omitempty"`
	Available        bool       `json:"available"`
	Architecture     string     `json:"architecture"`
	BundleFile       string     `json:"bundle_file,omitempty"`
	BundleSizeBytes  int64      `json:"bundle_size_bytes,omitempty"`
	PublishedAt      *time.Time `json:"published_at,omitempty"`
	Notes            string     `json:"notes,omitempty"`
}

type Service struct {
	currentVersion string
	architecture   string
	client         *http.Client
}

func New(currentVersion string) *Service {
	return &Service{
		currentVersion: strings.TrimSpace(currentVersion),
		architecture:   runtime.GOARCH,
		client:         &http.Client{Timeout: 15 * time.Second},
	}
}

func (s *Service) Check(ctx context.Context) (ReleaseStatus, error) {
	base := ReleaseStatus{
		CurrentVersion: s.currentVersion,
		Architecture:   s.architecture,
	}
	releases, err := s.fetchReleases(ctx)
	if err != nil {
		return ReleaseStatus{}, err
	}

	var best *ReleaseStatus
	for _, release := range releases {
		if release.Draft {
			continue
		}
		version := strings.TrimPrefix(strings.TrimSpace(release.TagName), "v")
		if version == "" || compareVersions(version, s.currentVersion) <= 0 {
			continue
		}

		bundleName := fmt.Sprintf("home-ai-core-update_%s_%s.tar.gz", version, s.architecture)
		checksumName := bundleName + ".sha256"
		var bundle *githubAsset
		hasChecksum := false
		for i := range release.Assets {
			asset := &release.Assets[i]
			switch asset.Name {
			case bundleName:
				bundle = asset
			case checksumName:
				hasChecksum = asset.BrowserDownloadURL != ""
			}
		}
		if bundle == nil || bundle.BrowserDownloadURL == "" || bundle.Size <= 0 || !hasChecksum {
			continue
		}

		published := release.PublishedAt
		notes := release.Body
		if len(notes) > 8000 {
			notes = notes[:8000]
		}
		item := ReleaseStatus{
			CurrentVersion:   s.currentVersion,
			AvailableVersion: version,
			Available:        true,
			Architecture:     s.architecture,
			BundleFile:       bundleName,
			BundleSizeBytes:  bundle.Size,
			PublishedAt:      &published,
			Notes:            notes,
		}
		if best == nil || compareVersions(item.AvailableVersion, best.AvailableVersion) > 0 {
			copy := item
			best = &copy
		}
	}
	if best == nil {
		return base, nil
	}
	return *best, nil
}

type githubRelease struct {
	TagName     string        `json:"tag_name"`
	Draft       bool          `json:"draft"`
	PublishedAt time.Time     `json:"published_at"`
	Body        string        `json:"body"`
	Assets      []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

func (s *Service) fetchReleases(ctx context.Context) ([]githubRelease, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releasesURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", defaultUserAgent+"/"+s.currentVersion)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("check GitHub releases: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("check GitHub releases: HTTP %d", resp.StatusCode)
	}

	decoder := json.NewDecoder(http.MaxBytesReader(nil, resp.Body, maxReleaseBytes))
	var releases []githubRelease
	if err := decoder.Decode(&releases); err != nil {
		return nil, fmt.Errorf("decode GitHub releases: %w", err)
	}
	return releases, nil
}

func compareVersions(left, right string) int {
	lMain, lPre := splitVersion(left)
	rMain, rPre := splitVersion(right)

	lParts := numericVersion(lMain)
	rParts := numericVersion(rMain)
	max := len(lParts)
	if len(rParts) > max {
		max = len(rParts)
	}
	for i := 0; i < max; i++ {
		var l, r int
		if i < len(lParts) {
			l = lParts[i]
		}
		if i < len(rParts) {
			r = rParts[i]
		}
		if l < r {
			return -1
		}
		if l > r {
			return 1
		}
	}
	if lPre == rPre {
		return 0
	}
	if lPre == "" {
		return 1
	}
	if rPre == "" {
		return -1
	}
	if lPre < rPre {
		return -1
	}
	return 1
}

func splitVersion(value string) (string, string) {
	value = strings.TrimPrefix(strings.TrimSpace(value), "v")
	main, pre, ok := strings.Cut(value, "-")
	if !ok {
		return main, ""
	}
	return main, pre
}

func numericVersion(value string) []int {
	parts := strings.Split(value, ".")
	result := make([]int, len(parts))
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return []int{0}
		}
		result[i] = n
	}
	return result
}
