package windowsclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultWindowsClientReleasesURL = "https://api.github.com/repos/DeadSoulf/home-ai-core/releases?per_page=20"
	maxReleaseMetadataBytes         = 4 << 20
	maxReleaseChecksumBytes         = 64 << 10
	maxWindowsClientBytes           = 256 << 20
)

var ErrNoCompatibleWindowsClientRelease = errors.New("no compatible Home-AI Windows client release found")

type WindowsClientRelease struct {
	Version        string
	TagName        string
	ExecutableName string
	ExecutableURL  string
	ExecutableSize int64
	ChecksumName   string
	ChecksumURL    string
}

type githubRelease struct {
	TagName string               `json:"tag_name"`
	Draft   bool                 `json:"draft"`
	Assets  []githubReleaseAsset `json:"assets"`
}

type githubReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

func DiscoverLatestWindowsClientRelease(ctx context.Context, client *http.Client) (WindowsClientRelease, error) {
	return discoverLatestWindowsClientReleaseFromURL(ctx, client, defaultWindowsClientReleasesURL)
}

func discoverLatestWindowsClientReleaseFromURL(ctx context.Context, client *http.Client, releasesURL string) (WindowsClientRelease, error) {
	if err := validateReleaseURL(releasesURL); err != nil {
		return WindowsClientRelease{}, fmt.Errorf("validate releases URL: %w", err)
	}
	data, err := getLimited(ctx, client, releasesURL, maxReleaseMetadataBytes, "application/vnd.github+json")
	if err != nil {
		return WindowsClientRelease{}, fmt.Errorf("download release metadata: %w", err)
	}
	var releases []githubRelease
	if err := json.Unmarshal(data, &releases); err != nil {
		return WindowsClientRelease{}, fmt.Errorf("decode release metadata: %w", err)
	}
	for _, release := range releases {
		if release.Draft {
			continue
		}
		version := strings.TrimPrefix(strings.TrimSpace(release.TagName), "v")
		if version == "" {
			continue
		}
		executableName := fmt.Sprintf("home-ai-windows-client_%s_amd64.exe", version)
		checksumName := executableName + ".sha256"
		var executable, checksum *githubReleaseAsset
		for i := range release.Assets {
			asset := &release.Assets[i]
			switch asset.Name {
			case executableName:
				executable = asset
			case checksumName:
				checksum = asset
			}
		}
		if executable == nil || checksum == nil {
			continue
		}
		if executable.Size <= 0 || executable.Size > maxWindowsClientBytes {
			continue
		}
		if err := validateReleaseURL(executable.BrowserDownloadURL); err != nil {
			continue
		}
		if err := validateReleaseURL(checksum.BrowserDownloadURL); err != nil {
			continue
		}
		return WindowsClientRelease{
			Version:        version,
			TagName:        release.TagName,
			ExecutableName: executableName,
			ExecutableURL:  executable.BrowserDownloadURL,
			ExecutableSize: executable.Size,
			ChecksumName:   checksumName,
			ChecksumURL:    checksum.BrowserDownloadURL,
		}, nil
	}
	return WindowsClientRelease{}, ErrNoCompatibleWindowsClientRelease
}

func DownloadWindowsClientRelease(ctx context.Context, client *http.Client, release WindowsClientRelease) (string, string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", "", fmt.Errorf("locate Windows client update cache: %w", err)
	}
	destination := filepath.Join(base, "HomeAI", "updates", release.Version)
	return downloadWindowsClientReleaseToDir(ctx, client, release, destination)
}

func downloadWindowsClientReleaseToDir(ctx context.Context, client *http.Client, release WindowsClientRelease, destination string) (string, string, error) {
	if err := validateWindowsClientRelease(release); err != nil {
		return "", "", err
	}
	checksumData, err := getLimited(ctx, client, release.ChecksumURL, maxReleaseChecksumBytes, "application/octet-stream")
	if err != nil {
		return "", "", fmt.Errorf("download Windows client checksum: %w", err)
	}
	expectedHash, err := parseReleaseChecksum(string(checksumData), release.ExecutableName)
	if err != nil {
		return "", "", fmt.Errorf("parse Windows client checksum: %w", err)
	}
	if err := os.MkdirAll(destination, 0o700); err != nil {
		return "", "", fmt.Errorf("create Windows client update cache: %w", err)
	}
	target := filepath.Join(destination, release.ExecutableName)
	if existingHash, err := hashRegularFile(target); err == nil && existingHash == expectedHash {
		return target, expectedHash, nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", "", fmt.Errorf("inspect cached Windows client: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, release.ExecutableURL, nil)
	if err != nil {
		return "", "", fmt.Errorf("create Windows client download request: %w", err)
	}
	req.Header.Set("User-Agent", "Home-AI-Windows-Client")
	resp, err := httpClient(client).Do(req)
	if err != nil {
		return "", "", fmt.Errorf("download Windows client: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("download Windows client: unexpected HTTP status %s", resp.Status)
	}
	if resp.ContentLength > maxWindowsClientBytes {
		return "", "", fmt.Errorf("Windows client is too large: %d bytes", resp.ContentLength)
	}
	if release.ExecutableSize > 0 && resp.ContentLength >= 0 && resp.ContentLength != release.ExecutableSize {
		return "", "", fmt.Errorf("Windows client size = %d, release metadata says %d", resp.ContentLength, release.ExecutableSize)
	}

	temp, err := os.CreateTemp(destination, ".home-ai-windows-client-*")
	if err != nil {
		return "", "", fmt.Errorf("create Windows client download temp file: %w", err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0o700); err != nil {
		_ = temp.Close()
		return "", "", fmt.Errorf("set Windows client cache permissions: %w", err)
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(temp, hash), io.LimitReader(resp.Body, maxWindowsClientBytes+1))
	if copyErr != nil {
		_ = temp.Close()
		return "", "", fmt.Errorf("save Windows client: %w", copyErr)
	}
	if written > maxWindowsClientBytes {
		_ = temp.Close()
		return "", "", errors.New("Windows client exceeds maximum supported size")
	}
	if release.ExecutableSize > 0 && written != release.ExecutableSize {
		_ = temp.Close()
		return "", "", fmt.Errorf("downloaded Windows client size = %d, want %d", written, release.ExecutableSize)
	}
	actualHash := hex.EncodeToString(hash.Sum(nil))
	if actualHash != expectedHash {
		_ = temp.Close()
		return "", "", fmt.Errorf("Windows client SHA-256 mismatch: got %s, want %s", actualHash, expectedHash)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return "", "", fmt.Errorf("sync Windows client download: %w", err)
	}
	if err := temp.Close(); err != nil {
		return "", "", fmt.Errorf("close Windows client download: %w", err)
	}
	if err := replaceQueueFile(tempName, target); err != nil {
		return "", "", fmt.Errorf("activate verified Windows client download: %w", err)
	}
	verifiedHash, err := hashRegularFile(target)
	if err != nil {
		return "", "", fmt.Errorf("verify cached Windows client: %w", err)
	}
	if verifiedHash != expectedHash {
		return "", "", errors.New("cached Windows client SHA-256 changed after activation")
	}
	return target, expectedHash, nil
}

func validateWindowsClientRelease(release WindowsClientRelease) error {
	if strings.TrimSpace(release.Version) == "" {
		return errors.New("release version is required")
	}
	expectedName := fmt.Sprintf("home-ai-windows-client_%s_amd64.exe", release.Version)
	if release.ExecutableName != expectedName {
		return fmt.Errorf("unexpected Windows client asset name %q", release.ExecutableName)
	}
	if release.ChecksumName != expectedName+".sha256" {
		return fmt.Errorf("unexpected Windows client checksum name %q", release.ChecksumName)
	}
	if filepath.Base(release.ExecutableName) != release.ExecutableName || filepath.Base(release.ChecksumName) != release.ChecksumName {
		return errors.New("release asset names must not contain paths")
	}
	if release.ExecutableSize <= 0 || release.ExecutableSize > maxWindowsClientBytes {
		return fmt.Errorf("invalid Windows client size %d", release.ExecutableSize)
	}
	if err := validateReleaseURL(release.ExecutableURL); err != nil {
		return fmt.Errorf("validate Windows client URL: %w", err)
	}
	if err := validateReleaseURL(release.ChecksumURL); err != nil {
		return fmt.Errorf("validate Windows client checksum URL: %w", err)
	}
	return nil
}

func parseReleaseChecksum(data, executableName string) (string, error) {
	var found string
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[1], "*")
		if filepath.Base(filepath.Clean(name)) != executableName {
			continue
		}
		rawHash := strings.ToLower(fields[0])
		decoded, err := hex.DecodeString(rawHash)
		if err != nil || len(decoded) != sha256.Size {
			return "", errors.New("checksum is not a SHA-256 digest")
		}
		if found != "" && found != rawHash {
			return "", errors.New("checksum file contains conflicting digests")
		}
		found = rawHash
	}
	if found == "" {
		return "", fmt.Errorf("checksum for %s not found", executableName)
	}
	return found, nil
}

func getLimited(ctx context.Context, client *http.Client, rawURL string, limit int64, accept string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Home-AI-Windows-Client")
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := httpClient(client).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP status %s", resp.Status)
	}
	if resp.ContentLength > limit {
		return nil, fmt.Errorf("response is too large: %d bytes", resp.ContentLength)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("response exceeds maximum supported size")
	}
	return data, nil
}

func httpClient(client *http.Client) *http.Client {
	if client != nil {
		return client
	}
	return http.DefaultClient
}

func validateReleaseURL(rawURL string) error {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return err
	}
	if parsed.User != nil || parsed.Host == "" {
		return errors.New("release URL must be an absolute URL without user information")
	}
	if parsed.Scheme == "https" {
		return nil
	}
	if parsed.Scheme == "http" && isLoopbackHost(parsed.Hostname()) {
		return nil
	}
	return errors.New("release URL must use HTTPS")
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
