package windowsclient

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
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	defaultWindowsReleaseAPI = "https://api.github.com/repos/DeadSoulf/home-ai-core/releases?per_page=20"
	maxReleaseResponseBytes   = 2 << 20
	maxChecksumBytes          = 4 << 10
	maxWindowsClientBytes     = 128 << 20
)

var (
	ErrWindowsClientVersionUnknown = errors.New("Windows client release version is unknown")
	devVersionPattern              = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)-dev$`)
)

type WindowsClientUpdate struct {
	Version       string
	ExecutableURL string
	ChecksumURL   string
}

type DownloadedWindowsClientUpdate struct {
	Version string
	Path    string
	SHA256  string
}

type WindowsReleaseSource struct {
	APIURL     string
	HTTPClient *http.Client
	allowHTTP  bool
}

type windowsRelease struct {
	TagName    string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

type devVersion struct {
	major int
	minor int
	patch int
}

func DefaultWindowsReleaseSource() WindowsReleaseSource {
	return WindowsReleaseSource{APIURL: defaultWindowsReleaseAPI}
}

func (source WindowsReleaseSource) FindUpdate(ctx context.Context, currentVersion string) (*WindowsClientUpdate, error) {
	current, err := parseDevVersion(currentVersion)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrWindowsClientVersionUnknown, err)
	}
	apiURL := strings.TrimSpace(source.APIURL)
	if apiURL == "" {
		apiURL = defaultWindowsReleaseAPI
	}
	if err := source.validateURL(apiURL); err != nil {
		return nil, fmt.Errorf("invalid release API URL: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "Home-AI-Windows-Client")

	response, err := source.client().Do(request)
	if err != nil {
		return nil, fmt.Errorf("check Windows client releases: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("release API returned HTTP %d", response.StatusCode)
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxReleaseResponseBytes+1))
	var releases []windowsRelease
	if err := decoder.Decode(&releases); err != nil {
		return nil, fmt.Errorf("decode release list: %w", err)
	}

	var best *WindowsClientUpdate
	var bestVersion devVersion
	for _, release := range releases {
		if release.Draft || !release.Prerelease {
			continue
		}
		parsed, err := parseDevVersion(release.TagName)
		if err != nil || compareDevVersion(parsed, current) <= 0 {
			continue
		}
		version := strings.TrimPrefix(strings.TrimSpace(release.TagName), "v")
		exeName := "home-ai-windows-client_" + version + "_amd64.exe"
		shaName := exeName + ".sha256"
		var exeURL, shaURL string
		for _, asset := range release.Assets {
			switch asset.Name {
			case exeName:
				exeURL = asset.BrowserDownloadURL
			case shaName:
				shaURL = asset.BrowserDownloadURL
			}
		}
		if exeURL == "" || shaURL == "" {
			continue
		}
		if err := source.validateURL(exeURL); err != nil {
			continue
		}
		if err := source.validateURL(shaURL); err != nil {
			continue
		}
		if best == nil || compareDevVersion(parsed, bestVersion) > 0 {
			bestVersion = parsed
			best = &WindowsClientUpdate{
				Version:       version,
				ExecutableURL: exeURL,
				ChecksumURL:   shaURL,
			}
		}
	}
	return best, nil
}

func (source WindowsReleaseSource) Download(
	ctx context.Context,
	update WindowsClientUpdate,
	destinationDir string,
) (DownloadedWindowsClientUpdate, error) {
	if _, err := parseDevVersion(update.Version); err != nil {
		return DownloadedWindowsClientUpdate{}, fmt.Errorf("invalid update version: %w", err)
	}
	if err := source.validateURL(update.ChecksumURL); err != nil {
		return DownloadedWindowsClientUpdate{}, fmt.Errorf("invalid checksum URL: %w", err)
	}
	if err := source.validateURL(update.ExecutableURL); err != nil {
		return DownloadedWindowsClientUpdate{}, fmt.Errorf("invalid executable URL: %w", err)
	}
	expected, err := source.downloadChecksum(ctx, update.ChecksumURL)
	if err != nil {
		return DownloadedWindowsClientUpdate{}, err
	}
	destinationDir = strings.TrimSpace(destinationDir)
	if destinationDir == "" {
		return DownloadedWindowsClientUpdate{}, errors.New("update destination directory is required")
	}
	destinationDir, err = filepath.Abs(destinationDir)
	if err != nil {
		return DownloadedWindowsClientUpdate{}, fmt.Errorf("resolve update directory: %w", err)
	}
	if err := os.MkdirAll(destinationDir, 0o700); err != nil {
		return DownloadedWindowsClientUpdate{}, fmt.Errorf("create update directory: %w", err)
	}
	dir, err := filepath.EvalSymlinks(destinationDir)
	if err != nil {
		return DownloadedWindowsClientUpdate{}, fmt.Errorf("resolve update directory: %w", err)
	}
	filename := "home-ai-windows-client_" + update.Version + "_amd64.exe"
	target := filepath.Join(dir, filename)
	if info, err := os.Lstat(target); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return DownloadedWindowsClientUpdate{}, errors.New("update target must be a real regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return DownloadedWindowsClientUpdate{}, fmt.Errorf("inspect update target: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, update.ExecutableURL, nil)
	if err != nil {
		return DownloadedWindowsClientUpdate{}, err
	}
	request.Header.Set("Accept", "application/octet-stream")
	request.Header.Set("User-Agent", "Home-AI-Windows-Client")
	response, err := source.client().Do(request)
	if err != nil {
		return DownloadedWindowsClientUpdate{}, fmt.Errorf("download Windows client: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return DownloadedWindowsClientUpdate{}, fmt.Errorf("Windows client download returned HTTP %d", response.StatusCode)
	}
	if response.ContentLength > maxWindowsClientBytes {
		return DownloadedWindowsClientUpdate{}, errors.New("Windows client update exceeds size limit")
	}
	temp, err := os.CreateTemp(dir, ".home-ai-client-update-*")
	if err != nil {
		return DownloadedWindowsClientUpdate{}, fmt.Errorf("create update temp file: %w", err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0o700); err != nil {
		_ = temp.Close()
		return DownloadedWindowsClientUpdate{}, err
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(temp, hash), io.LimitReader(response.Body, maxWindowsClientBytes+1))
	if copyErr != nil {
		_ = temp.Close()
		return DownloadedWindowsClientUpdate{}, fmt.Errorf("download Windows client bytes: %w", copyErr)
	}
	if written > maxWindowsClientBytes {
		_ = temp.Close()
		return DownloadedWindowsClientUpdate{}, errors.New("Windows client update exceeds size limit")
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(actual, expected) {
		_ = temp.Close()
		return DownloadedWindowsClientUpdate{}, errors.New("Windows client update SHA-256 does not match published checksum")
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return DownloadedWindowsClientUpdate{}, fmt.Errorf("sync update download: %w", err)
	}
	if err := temp.Close(); err != nil {
		return DownloadedWindowsClientUpdate{}, fmt.Errorf("close update download: %w", err)
	}
	if err := replaceQueueFile(tempName, target); err != nil {
		return DownloadedWindowsClientUpdate{}, fmt.Errorf("activate downloaded update: %w", err)
	}
	verified, err := hashRegularFile(target)
	if err != nil {
		return DownloadedWindowsClientUpdate{}, fmt.Errorf("verify downloaded update: %w", err)
	}
	if !strings.EqualFold(verified, expected) {
		return DownloadedWindowsClientUpdate{}, errors.New("downloaded update changed after activation")
	}
	return DownloadedWindowsClientUpdate{
		Version: update.Version,
		Path:    target,
		SHA256:  strings.ToLower(expected),
	}, nil
}

func (source WindowsReleaseSource) downloadChecksum(ctx context.Context, rawURL string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Accept", "text/plain")
	request.Header.Set("User-Agent", "Home-AI-Windows-Client")
	response, err := source.client().Do(request)
	if err != nil {
		return "", fmt.Errorf("download Windows client checksum: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("checksum download returned HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxChecksumBytes+1))
	if err != nil {
		return "", fmt.Errorf("read checksum: %w", err)
	}
	if len(data) > maxChecksumBytes {
		return "", errors.New("checksum response exceeds size limit")
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return "", errors.New("checksum response is empty")
	}
	checksum := strings.ToLower(fields[0])
	decoded, err := hex.DecodeString(checksum)
	if err != nil || len(decoded) != sha256.Size {
		return "", errors.New("published checksum is not a SHA-256 value")
	}
	return checksum, nil
}

func (source WindowsReleaseSource) client() *http.Client {
	if source.HTTPClient != nil {
		copy := *source.HTTPClient
		if !source.allowHTTP {
			copy.CheckRedirect = secureReleaseRedirect
		}
		if copy.Timeout <= 0 {
			copy.Timeout = 2 * time.Minute
		}
		return &copy
	}
	client := &http.Client{Timeout: 2 * time.Minute}
	if !source.allowHTTP {
		client.CheckRedirect = secureReleaseRedirect
	}
	return client
}

func (source WindowsReleaseSource) validateURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return err
	}
	if source.allowHTTP {
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return errors.New("release URL must use HTTP or HTTPS")
		}
		return nil
	}
	if parsed.Scheme != "https" {
		return errors.New("release URL must use HTTPS")
	}
	if !allowedReleaseHost(parsed.Hostname()) {
		return fmt.Errorf("release host %q is not allowed", parsed.Hostname())
	}
	return nil
}

func secureReleaseRedirect(request *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("too many release download redirects")
	}
	if request.URL.Scheme != "https" || !allowedReleaseHost(request.URL.Hostname()) {
		return fmt.Errorf("release redirect host %q is not allowed", request.URL.Hostname())
	}
	return nil
}

func allowedReleaseHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	return host == "api.github.com" ||
		host == "github.com" ||
		strings.HasSuffix(host, ".githubusercontent.com")
}

func parseDevVersion(raw string) (devVersion, error) {
	match := devVersionPattern.FindStringSubmatch(strings.TrimSpace(raw))
	if match == nil {
		return devVersion{}, fmt.Errorf("version %q is not x.y.z-dev", raw)
	}
	values := make([]int, 3)
	for i := range values {
		value, err := strconv.Atoi(match[i+1])
		if err != nil {
			return devVersion{}, err
		}
		values[i] = value
	}
	return devVersion{major: values[0], minor: values[1], patch: values[2]}, nil
}

func compareDevVersion(left, right devVersion) int {
	if left.major != right.major {
		if left.major < right.major {
			return -1
		}
		return 1
	}
	if left.minor != right.minor {
		if left.minor < right.minor {
			return -1
		}
		return 1
	}
	if left.patch < right.patch {
		return -1
	}
	if left.patch > right.patch {
		return 1
	}
	return 0
}
