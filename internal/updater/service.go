package updater

import (
	"archive/tar"
	"compress/gzip"
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
	"sync"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
)

const (
	releasesURL       = "https://api.github.com/repos/DeadSoulf/home-ai-core/releases?per_page=20"
	maxReleaseBytes   = 1 << 20
	maxChecksumBytes  = 4096
	maxBundleBytes    = 512 << 20
	maxExtractedBytes = 768 << 20
	maxBundleFiles    = 20000
	defaultUserAgent  = "Home-AI-Core"
)

var (
	ErrNoUpdate = errors.New("no update available")
	ErrBusy     = errors.New("update operation already in progress")
)

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

type candidate struct {
	ReleaseStatus
	BundleURL   string
	ChecksumURL string
}

type Service struct {
	currentVersion string
	architecture   string
	stateDir       string
	client         *http.Client

	opMu    sync.Mutex
	stateMu sync.RWMutex
	state   State
}

func New(currentVersion, stateDir string) *Service {
	currentVersion = strings.TrimSpace(currentVersion)
	service := &Service{
		currentVersion: currentVersion,
		architecture:   runtime.GOARCH,
		stateDir:       stateDir,
		client:         &http.Client{Timeout: 30 * time.Second},
		state:          NewState(currentVersion),
	}
	service.loadPersistedState()
	service.reconcileInstallResult()
	return service
}

func (s *Service) State() State {
	s.reconcileInstallResult()
	return s.snapshotState()
}

func (s *Service) snapshotState() State {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.state
}

func (s *Service) Check(ctx context.Context) (ReleaseStatus, error) {
	s.setState(State{
		Phase:          PhaseChecking,
		CurrentVersion: s.currentVersion,
		UpdatedAt:      time.Now().UTC(),
	})

	item, err := s.findCandidate(ctx, "")
	if errors.Is(err, ErrNoUpdate) {
		status := ReleaseStatus{
			CurrentVersion: s.currentVersion,
			Architecture:   s.architecture,
			Available:      false,
		}
		s.setState(State{
			Phase:          PhaseIdle,
			CurrentVersion: s.currentVersion,
			Message:        "Home-AI-Core is up to date",
			UpdatedAt:      time.Now().UTC(),
		})
		return status, nil
	}
	if err != nil {
		s.failState(err)
		return ReleaseStatus{}, err
	}

	current := s.snapshotState()
	if current.Phase == PhaseReady && current.AvailableVersion == item.AvailableVersion {
		current.CurrentVersion = s.currentVersion
		current.PublishedAt = item.PublishedAt
		current.BundleSizeBytes = item.BundleSizeBytes
		current.UpdatedAt = time.Now().UTC()
		s.setState(current)
	} else {
		s.setState(State{
			Phase:            PhaseAvailable,
			CurrentVersion:   s.currentVersion,
			AvailableVersion: item.AvailableVersion,
			Message:          "Update available",
			UpdatedAt:        time.Now().UTC(),
			PublishedAt:      item.PublishedAt,
			BundleSizeBytes:  item.BundleSizeBytes,
		})
	}
	return item.ReleaseStatus, nil
}

func (s *Service) Download(ctx context.Context, version string) (State, error) {
	if !s.opMu.TryLock() {
		return s.State(), ErrBusy
	}
	defer s.opMu.Unlock()

	version = strings.TrimSpace(version)
	item, err := s.findCandidate(ctx, version)
	if err != nil {
		s.failState(err)
		return s.State(), err
	}

	s.setState(State{
		Phase:            PhaseDownloading,
		CurrentVersion:   s.currentVersion,
		AvailableVersion: item.AvailableVersion,
		ProgressPercent:  5,
		Message:          "Reading update checksum",
		UpdatedAt:        time.Now().UTC(),
		PublishedAt:      item.PublishedAt,
		BundleSizeBytes:  item.BundleSizeBytes,
	})

	expectedHash, err := s.fetchChecksum(ctx, item.ChecksumURL, item.BundleFile)
	if err != nil {
		s.failState(err)
		return s.State(), err
	}

	updateDir := filepath.Join(s.stateDir, "update")
	if err := os.MkdirAll(updateDir, 0o700); err != nil {
		s.failState(err)
		return s.State(), err
	}
	archivePath := filepath.Join(updateDir, item.BundleFile)

	s.updateProgress(15, "Downloading update bundle")
	if err := s.downloadArchive(ctx, item.BundleURL, archivePath, item.BundleSizeBytes, expectedHash); err != nil {
		s.failState(err)
		return s.State(), err
	}

	s.updateProgress(70, "Verifying update bundle")
	preparedDir := filepath.Join(updateDir, "prepared-"+safeVersion(version))
	if err := os.RemoveAll(preparedDir); err != nil {
		s.failState(err)
		return s.State(), err
	}
	if err := os.MkdirAll(preparedDir, 0o700); err != nil {
		s.failState(err)
		return s.State(), err
	}
	if err := extractAndVerifyBundle(archivePath, preparedDir, version, s.architecture); err != nil {
		_ = os.RemoveAll(preparedDir)
		s.failState(err)
		return s.State(), err
	}

	ready := State{
		Phase:            PhaseReady,
		CurrentVersion:   s.currentVersion,
		AvailableVersion: version,
		ProgressPercent:  100,
		Message:          "Update bundle downloaded and verified",
		UpdatedAt:        time.Now().UTC(),
		PublishedAt:      item.PublishedAt,
		BundleSizeBytes:  item.BundleSizeBytes,
	}
	s.setState(ready)
	return ready, nil
}

func (s *Service) Install(ctx context.Context, version string) (State, error) {
	if !s.opMu.TryLock() {
		return s.State(), ErrBusy
	}
	defer s.opMu.Unlock()

	version = strings.TrimSpace(version)
	state := s.snapshotState()
	if state.Phase != PhaseReady || state.AvailableVersion != version {
		return state, errors.New("requested update is not downloaded and ready")
	}

	preparedDir := filepath.Join(s.stateDir, "update", "prepared-"+safeVersion(version))
	if _, err := VerifyPreparedBundle(preparedDir, version); err != nil {
		s.failState(err)
		return s.snapshotState(), err
	}

	state.Phase = PhaseInstalling
	state.ProgressPercent = 100
	state.Message = "Starting privileged update installation"
	state.Error = ""
	state.UpdatedAt = time.Now().UTC()
	s.setState(state)

	if err := callUpdaterHelper(ctx, version); err != nil {
		s.failState(err)
		return s.snapshotState(), err
	}

	state = s.snapshotState()
	state.Phase = PhaseRestarting
	state.Message = "Update accepted; Home-AI-Core is restarting"
	state.UpdatedAt = time.Now().UTC()
	s.setState(state)
	return state, nil
}

func (s *Service) findCandidate(ctx context.Context, requestedVersion string) (candidate, error) {
	releases, err := s.fetchReleases(ctx)
	if err != nil {
		return candidate{}, err
	}

	var best *candidate
	for _, release := range releases {
		if release.Draft {
			continue
		}
		version := strings.TrimPrefix(strings.TrimSpace(release.TagName), "v")
		if version == "" || compareVersions(version, s.currentVersion) <= 0 {
			continue
		}
		if requestedVersion != "" && version != requestedVersion {
			continue
		}

		bundleName := fmt.Sprintf("home-ai-core-update_%s_%s.tar.gz", version, s.architecture)
		checksumName := bundleName + ".sha256"
		var bundle, checksum *githubAsset
		for i := range release.Assets {
			asset := &release.Assets[i]
			switch asset.Name {
			case bundleName:
				bundle = asset
			case checksumName:
				checksum = asset
			}
		}
		if bundle == nil || checksum == nil || bundle.BrowserDownloadURL == "" || checksum.BrowserDownloadURL == "" {
			continue
		}
		if bundle.Size <= 0 || bundle.Size > maxBundleBytes {
			continue
		}

		published := release.PublishedAt
		notes := release.Body
		if len(notes) > 8000 {
			notes = notes[:8000]
		}
		item := candidate{
			ReleaseStatus: ReleaseStatus{
				CurrentVersion:   s.currentVersion,
				AvailableVersion: version,
				Available:        true,
				Architecture:     s.architecture,
				BundleFile:       bundleName,
				BundleSizeBytes:  bundle.Size,
				PublishedAt:      &published,
				Notes:            notes,
			},
			BundleURL:   bundle.BrowserDownloadURL,
			ChecksumURL: checksum.BrowserDownloadURL,
		}
		if requestedVersion != "" {
			return item, nil
		}
		if best == nil || compareVersions(item.AvailableVersion, best.AvailableVersion) > 0 {
			copy := item
			best = &copy
		}
	}
	if best == nil {
		return candidate{}, ErrNoUpdate
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

	decoder := json.NewDecoder(io.LimitReader(resp.Body, maxReleaseBytes+1))
	var releases []githubRelease
	if err := decoder.Decode(&releases); err != nil {
		return nil, fmt.Errorf("decode GitHub releases: %w", err)
	}
	return releases, nil
}

func (s *Service) fetchChecksum(ctx context.Context, url, expectedFile string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", defaultUserAgent+"/"+s.currentVersion)
	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download checksum: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download checksum: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxChecksumBytes))
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(data))
	if len(fields) < 2 || strings.TrimPrefix(fields[1], "*") != expectedFile {
		return "", errors.New("invalid update checksum file")
	}
	hash := strings.ToLower(fields[0])
	if len(hash) != sha256.Size*2 {
		return "", errors.New("invalid update checksum")
	}
	if _, err := hex.DecodeString(hash); err != nil {
		return "", errors.New("invalid update checksum")
	}
	return hash, nil
}

func (s *Service) downloadArchive(ctx context.Context, url, target string, expectedSize int64, expectedHash string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", defaultUserAgent+"/"+s.currentVersion)
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("download update bundle: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download update bundle: HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > 0 && resp.ContentLength != expectedSize {
		return errors.New("update bundle size does not match release metadata")
	}

	tmp := target + ".part"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(resp.Body, expectedSize+1))
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	if written != expectedSize {
		_ = os.Remove(tmp)
		return fmt.Errorf("update bundle size mismatch: got %d, want %d", written, expectedSize)
	}
	if actual := hex.EncodeToString(hash.Sum(nil)); actual != expectedHash {
		_ = os.Remove(tmp)
		return errors.New("update bundle SHA-256 mismatch")
	}
	return os.Rename(tmp, target)
}

func extractAndVerifyBundle(archivePath, targetDir, version, architecture string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()

	gz, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("open update gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	extracted := make(map[string]string)
	var total int64
	files := 0

	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read update archive: %w", err)
		}

		rawName := filepath.ToSlash(header.Name)
		name := filepath.ToSlash(filepath.Clean(header.Name))
		if name == "." || strings.HasPrefix(name, "../") || strings.HasPrefix(rawName, "/") {
			return errors.New("update archive contains unsafe path")
		}
		if header.Typeflag == tar.TypeDir {
			if rawName != name && rawName != name+"/" {
				return errors.New("update archive contains unsafe path")
			}
		} else if rawName != name {
			return errors.New("update archive contains unsafe path")
		}
		target := filepath.Join(targetDir, filepath.FromSlash(name))
		if !strings.HasPrefix(target, targetDir+string(os.PathSeparator)) {
			return errors.New("update archive path escapes staging directory")
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o700); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			files++
			if files > maxBundleFiles || header.Size < 0 {
				return errors.New("update archive exceeds file limits")
			}
			total += header.Size
			if total > maxExtractedBytes {
				return errors.New("update archive exceeds extracted size limit")
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
			if err != nil {
				return err
			}
			hash := sha256.New()
			written, copyErr := io.Copy(io.MultiWriter(out, hash), io.LimitReader(tr, header.Size+1))
			closeErr := out.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
			if written != header.Size {
				return errors.New("update archive file size mismatch")
			}
			extracted[name] = hex.EncodeToString(hash.Sum(nil))
		default:
			return errors.New("update archive contains unsupported entry type")
		}
	}

	manifestData, err := os.ReadFile(filepath.Join(targetDir, "manifest.json"))
	if err != nil {
		return errors.New("update manifest is missing")
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return fmt.Errorf("decode update manifest: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return err
	}
	if manifest.Version != version {
		return errors.New("update manifest version mismatch")
	}
	if manifest.Architecture != architecture {
		return errors.New("update manifest architecture mismatch")
	}

	expected := make(map[string]ManifestFile, len(manifest.Files))
	for _, entry := range manifest.Files {
		expected[entry.Path] = entry
		actualHash, ok := extracted[entry.Path]
		if !ok {
			return fmt.Errorf("update file %q is missing", entry.Path)
		}
		info, err := os.Stat(filepath.Join(targetDir, filepath.FromSlash(entry.Path)))
		if err != nil {
			return err
		}
		if info.Size() != entry.SizeBytes {
			return fmt.Errorf("update file %q size mismatch", entry.Path)
		}
		if actualHash != entry.SHA256 {
			return fmt.Errorf("update file %q checksum mismatch", entry.Path)
		}
	}
	for path := range extracted {
		if path == "manifest.json" {
			continue
		}
		if _, ok := expected[path]; !ok {
			return fmt.Errorf("unexpected file %q in update bundle", path)
		}
	}
	return nil
}

func (s *Service) setState(state State) {
	s.stateMu.Lock()
	s.state = state
	s.stateMu.Unlock()
	s.persistState(state)
}

func (s *Service) updateProgress(progress int, message string) {
	state := s.snapshotState()
	state.ProgressPercent = progress
	state.Message = message
	state.UpdatedAt = time.Now().UTC()
	s.setState(state)
}

func (s *Service) failState(err error) {
	state := s.snapshotState()
	state.Phase = PhaseFailed
	state.Error = err.Error()
	state.Message = "Update operation failed"
	state.UpdatedAt = time.Now().UTC()
	s.setState(state)
}

func (s *Service) loadPersistedState() {
	path := filepath.Join(s.stateDir, "update", "state.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return
	}
	state.CurrentVersion = s.currentVersion
	s.state = state
}

func (s *Service) reconcileInstallResult() {
	path := filepath.Join(s.stateDir, "update", "install-result.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var result updaterhelper.Result
	if err := json.Unmarshal(data, &result); err != nil {
		return
	}

	state := s.snapshotState()
	switch {
	case result.Status == "succeeded" && result.Version == s.currentVersion:
		if state.Phase == PhaseSucceeded && state.AvailableVersion == result.Version {
			return
		}
		state.Phase = PhaseSucceeded
		state.CurrentVersion = s.currentVersion
		state.AvailableVersion = result.Version
		state.ProgressPercent = 100
		state.Message = result.Message
		if state.Message == "" {
			state.Message = "Update installed successfully"
		}
		state.Error = ""
		state.UpdatedAt = time.Now().UTC()
		s.setState(state)
	case result.Status == "failed" && state.AvailableVersion == result.Version:
		if state.Phase == PhaseFailed && state.Error == result.Error {
			return
		}
		state.Phase = PhaseFailed
		state.Message = "Update installation failed; previous version restored"
		state.Error = result.Error
		state.UpdatedAt = time.Now().UTC()
		s.setState(state)
	}
}

func (s *Service) persistState(state State) {
	dir := filepath.Join(s.stateDir, "update")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return
	}
	data = append(data, '\n')
	tmp := filepath.Join(dir, "state.json.tmp")
	target := filepath.Join(dir, "state.json")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return
	}
	_ = os.Rename(tmp, target)
}

func safeVersion(value string) string {
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	return b.String()
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
