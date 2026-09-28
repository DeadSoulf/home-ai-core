package coreupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/jobs"
	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/state"
)

const (
	releasesURL       = "https://api.github.com/repos/DeadSoulf/home-ai-core/releases?per_page=20"
	updateManifest    = "home-ai-core-update.json"
	jobTypeInstall    = "core.update.install"
	maxManifestBytes  = 128 << 10
	maxPackageBytes   = 256 << 20
	defaultSocketPath = "/run/home-ai-core-update.sock"
)

var (
	ErrNoUpdate          = errors.New("no update available")
	ErrInvalidUpdateFeed = errors.New("invalid update feed")
)

type Status struct {
	CurrentVersion   string     `json:"current_version"`
	AvailableVersion string     `json:"available_version,omitempty"`
	Available        bool       `json:"available"`
	PublishedAt      *time.Time `json:"published_at,omitempty"`
	Notes            string     `json:"notes,omitempty"`
}

type packageManifest struct {
	File          string `json:"file"`
	SHA256        string `json:"sha256"`
	SizeBytes     int64  `json:"size_bytes"`
	DebianVersion string `json:"debian_version"`
}

type releaseManifest struct {
	SchemaVersion int                        `json:"schema_version"`
	Version       string                     `json:"version"`
	Packages      map[string]packageManifest `json:"packages"`
}

type candidate struct {
	Version       string
	PublishedAt   time.Time
	Notes         string
	URL           string
	File          string
	SHA256        string
	SizeBytes     int64
	DebianVersion string
	Architecture  string
}

type githubRelease struct {
	TagName     string        `json:"tag_name"`
	Draft       bool          `json:"draft"`
	Prerelease  bool          `json:"prerelease"`
	PublishedAt time.Time     `json:"published_at"`
	Body        string        `json:"body"`
	Assets      []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

type Service struct {
	currentVersion string
	stateDir       string
	architecture   string
	helperSocket   string
	httpClient     *http.Client
	jobs           *jobs.Service
}

func New(currentVersion, stateDir string, jobService *jobs.Service) (*Service, error) {
	service := &Service{
		currentVersion: currentVersion,
		stateDir:       stateDir,
		architecture:   runtime.GOARCH,
		helperSocket:   defaultSocketPath,
		httpClient:     &http.Client{Timeout: 20 * time.Second},
		jobs:           jobService,
	}
	if jobService != nil {
		if err := jobService.Register(jobTypeInstall, service.installHandler); err != nil {
			return nil, err
		}
	}
	return service, nil
}

func (s *Service) Check(ctx context.Context) (Status, error) {
	item, err := s.findCandidate(ctx)
	if errors.Is(err, ErrNoUpdate) {
		return Status{CurrentVersion: s.currentVersion, Available: false}, nil
	}
	if err != nil {
		return Status{}, err
	}
	published := item.PublishedAt
	return Status{
		CurrentVersion:   s.currentVersion,
		AvailableVersion: item.Version,
		Available:        true,
		PublishedAt:      &published,
		Notes:            item.Notes,
	}, nil
}

func (s *Service) Install(
	ctx context.Context,
	actor security.Actor,
	meta security.RequestContext,
) (state.JobRecord, error) {
	if s.jobs == nil {
		return state.JobRecord{}, errors.New("update job service is unavailable")
	}
	item, err := s.findCandidate(ctx)
	if err != nil {
		return state.JobRecord{}, err
	}
	return s.jobs.Submit(ctx, state.JobRecord{
		Type:          jobTypeInstall,
		ActorType:     actor.Type,
		ActorID:       actor.ID,
		RequestID:     meta.RequestID,
		CorrelationID: meta.CorrelationID,
		Input: map[string]any{
			"version":        item.Version,
			"package_url":    item.URL,
			"package_file":   item.File,
			"sha256":         item.SHA256,
			"size_bytes":     item.SizeBytes,
			"debian_version": item.DebianVersion,
			"architecture":   item.Architecture,
		},
	})
}

func (s *Service) findCandidate(ctx context.Context) (candidate, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releasesURL, nil)
	if err != nil {
		return candidate{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Home-AI-Core/"+s.currentVersion)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return candidate{}, fmt.Errorf("check GitHub releases: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return candidate{}, fmt.Errorf("check GitHub releases: HTTP %d", resp.StatusCode)
	}

	var releases []githubRelease
	if err := decodeBoundedJSON(resp.Body, maxManifestBytes, &releases); err != nil {
		return candidate{}, fmt.Errorf("decode GitHub releases: %w", err)
	}

	var best *candidate
	for _, release := range releases {
		if release.Draft {
			continue
		}
		version := strings.TrimPrefix(strings.TrimSpace(release.TagName), "v")
		if compareVersions(version, s.currentVersion) <= 0 {
			continue
		}

		item, err := s.candidateFromRelease(ctx, release, version)
		if err != nil {
			continue
		}
		if best == nil || compareVersions(item.Version, best.Version) > 0 {
			copy := item
			best = &copy
		}
	}
	if best == nil {
		return candidate{}, ErrNoUpdate
	}
	return *best, nil
}

func (s *Service) candidateFromRelease(ctx context.Context, release githubRelease, version string) (candidate, error) {
	var manifestAsset *githubAsset
	assets := make(map[string]githubAsset, len(release.Assets))
	for i := range release.Assets {
		asset := release.Assets[i]
		assets[asset.Name] = asset
		if asset.Name == updateManifest {
			copy := asset
			manifestAsset = &copy
		}
	}
	if manifestAsset == nil || manifestAsset.BrowserDownloadURL == "" {
		return candidate{}, ErrInvalidUpdateFeed
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestAsset.BrowserDownloadURL, nil)
	if err != nil {
		return candidate{}, err
	}
	req.Header.Set("User-Agent", "Home-AI-Core/"+s.currentVersion)
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return candidate{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return candidate{}, ErrInvalidUpdateFeed
	}

	var manifest releaseManifest
	if err := decodeBoundedJSON(resp.Body, maxManifestBytes, &manifest); err != nil {
		return candidate{}, ErrInvalidUpdateFeed
	}
	if manifest.SchemaVersion != 1 || manifest.Version != version {
		return candidate{}, ErrInvalidUpdateFeed
	}
	pkg, ok := manifest.Packages[s.architecture]
	if !ok {
		return candidate{}, ErrInvalidUpdateFeed
	}
	if !validPackageManifest(pkg) {
		return candidate{}, ErrInvalidUpdateFeed
	}
	asset, ok := assets[pkg.File]
	if !ok || asset.BrowserDownloadURL == "" || asset.Size != pkg.SizeBytes {
		return candidate{}, ErrInvalidUpdateFeed
	}

	notes := release.Body
	if len(notes) > 8000 {
		notes = notes[:8000]
	}
	return candidate{
		Version:       version,
		PublishedAt:   release.PublishedAt,
		Notes:         notes,
		URL:           asset.BrowserDownloadURL,
		File:          pkg.File,
		SHA256:        pkg.SHA256,
		SizeBytes:     pkg.SizeBytes,
		DebianVersion: pkg.DebianVersion,
		Architecture:  s.architecture,
	}, nil
}

func validPackageManifest(pkg packageManifest) bool {
	if pkg.SizeBytes <= 0 || pkg.SizeBytes > maxPackageBytes || filepath.Base(pkg.File) != pkg.File {
		return false
	}
	if len(pkg.SHA256) != sha256.Size*2 || strings.ToLower(pkg.SHA256) != pkg.SHA256 {
		return false
	}
	_, err := hex.DecodeString(pkg.SHA256)
	return err == nil && pkg.DebianVersion != ""
}

func (s *Service) installHandler(ctx context.Context, job state.JobRecord, reporter jobs.Reporter) (map[string]any, error) {
	target, err := stringInput(job.Input, "version")
	if err != nil {
		return nil, err
	}
	if compareVersions(s.currentVersion, target) >= 0 {
		return map[string]any{"version": s.currentVersion, "already_installed": true}, nil
	}

	url, err := stringInput(job.Input, "package_url")
	if err != nil {
		return nil, err
	}
	file, err := stringInput(job.Input, "package_file")
	if err != nil || filepath.Base(file) != file {
		return nil, errors.New("invalid update package file")
	}
	expectedHash, err := stringInput(job.Input, "sha256")
	if err != nil {
		return nil, err
	}
	debianVersion, err := stringInput(job.Input, "debian_version")
	if err != nil {
		return nil, err
	}
	architecture, err := stringInput(job.Input, "architecture")
	if err != nil || architecture != s.architecture {
		return nil, errors.New("update architecture mismatch")
	}
	size, err := int64Input(job.Input, "size_bytes")
	if err != nil || size <= 0 || size > maxPackageBytes {
		return nil, errors.New("invalid update package size")
	}

	if err := reporter.Progress(ctx, 500, "downloading update package"); err != nil {
		return nil, err
	}
	updateDir := filepath.Join(s.stateDir, "updates")
	if err := os.MkdirAll(updateDir, 0o700); err != nil {
		return nil, fmt.Errorf("create update staging directory: %w", err)
	}
	targetPath := filepath.Join(updateDir, file)
	if err := s.downloadPackage(ctx, url, targetPath, size, expectedHash, reporter); err != nil {
		return nil, err
	}

	if err := reporter.Progress(ctx, 7500, "installing update package"); err != nil {
		return nil, err
	}
	err = callHelper(ctx, s.helperSocket, helperRequest{
		Operation:     "install-core-update",
		PackagePath:   targetPath,
		SHA256:        expectedHash,
		Version:       target,
		DebianVersion: debianVersion,
		Architecture:  architecture,
	})
	if err != nil {
		return nil, err
	}

	// A successful package install normally restarts the Core from postinst
	// before this process can reach here. If it does not, the job remains
	// correct and reports that a restart is pending.
	_ = reporter.Progress(ctx, 9500, "update installed; restarting Home-AI-Core")
	return map[string]any{"version": target, "restart_pending": true}, nil
}

func (s *Service) downloadPackage(
	ctx context.Context,
	url, targetPath string,
	expectedSize int64,
	expectedHash string,
	reporter jobs.Reporter,
) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Home-AI-Core/"+s.currentVersion)
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("download update: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download update: HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > 0 && resp.ContentLength != expectedSize {
		return errors.New("update package size does not match release metadata")
	}

	tmp := targetPath + ".part"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	hash := sha256.New()
	limited := io.LimitReader(resp.Body, expectedSize+1)
	written, copyErr := io.Copy(io.MultiWriter(file, hash), limited)
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("download update: %w", copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	if written != expectedSize {
		_ = os.Remove(tmp)
		return fmt.Errorf("update package size mismatch: got %d, want %d", written, expectedSize)
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if actual != expectedHash {
		_ = os.Remove(tmp)
		return errors.New("update package SHA-256 mismatch")
	}
	if err := reporter.Progress(ctx, 6500, "update package verified"); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, targetPath); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func decodeBoundedJSON(reader io.Reader, max int64, target any) error {
	decoder := json.NewDecoder(io.LimitReader(reader, max+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("response must contain exactly one JSON value")
	}
	return nil
}

func stringInput(input map[string]any, key string) (string, error) {
	value, ok := input[key].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("job input %q is required", key)
	}
	return value, nil
}

func int64Input(input map[string]any, key string) (int64, error) {
	switch value := input[key].(type) {
	case float64:
		return int64(value), nil
	case int64:
		return value, nil
	case int:
		return int64(value), nil
	case json.Number:
		return value.Int64()
	case string:
		return strconv.ParseInt(value, 10, 64)
	default:
		return 0, fmt.Errorf("job input %q is invalid", key)
	}
}
