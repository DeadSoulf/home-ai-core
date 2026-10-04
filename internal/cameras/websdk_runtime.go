package cameras

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
)

const maxWebSDKArchiveSize int64 = 64 << 20

type WebSDKRuntimeStatus struct {
	Available bool   `json:"available"`
	Version   string `json:"version,omitempty"`
	Error     string `json:"error,omitempty"`
}

type WebSDKInstallResult struct {
	Installed bool                `json:"installed"`
	Version   string              `json:"version"`
	Status    WebSDKRuntimeStatus `json:"status"`
}

type WebSDKAsset struct {
	Name        string
	ContentType string
	Download    bool
	Data        []byte
}

var webSDKAssets = map[string]string{
	"jquery-1.7.1.min.js":        "demo/jquery-1.7.1.min.js",
	"webVideoCtrl.js":            "demo/codebase/webVideoCtrl.js",
	"jsVideoPlugin-1.0.0.min.js": "demo/codebase/jsVideoPlugin-1.0.0.min.js",
	"HCWebSDKPlugin.exe":          "demo/codebase/HCWebSDKPlugin.exe",
}

func (s *Service) webSDKStatus() WebSDKRuntimeStatus {
	if s == nil || s.webSDKRoot == "" {
		return WebSDKRuntimeStatus{Error: "WebSDK runtime directory is not configured"}
	}
	for name := range webSDKAssets {
		info, err := os.Stat(filepath.Join(s.webSDKRoot, name))
		if err != nil || info.IsDir() {
			return WebSDKRuntimeStatus{Error: "WebSDK V3.3.1 runtime is not installed"}
		}
	}
	return WebSDKRuntimeStatus{Available: true, Version: "3.3.1"}
}

func (s *Service) InstallWebSDK(reader io.Reader, size int64) (WebSDKInstallResult, error) {
	if s == nil || s.webSDKRoot == "" {
		return WebSDKInstallResult{}, errors.New("WebSDK runtime directory is not configured")
	}
	if size <= 0 || size > maxWebSDKArchiveSize {
		return WebSDKInstallResult{}, fmt.Errorf("WebSDK archive must be between 1 byte and %d bytes", maxWebSDKArchiveSize)
	}

	tmp, err := os.CreateTemp("", "home-ai-websdk-*.zip")
	if err != nil {
		return WebSDKInstallResult{}, err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err = io.CopyN(tmp, reader, size); err != nil {
		_ = tmp.Close()
		return WebSDKInstallResult{}, fmt.Errorf("read WebSDK archive: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return WebSDKInstallResult{}, err
	}

	zr, err := zip.OpenReader(tmpName)
	if err != nil {
		return WebSDKInstallResult{}, fmt.Errorf("open WebSDK archive: %w", err)
	}
	defer zr.Close()

	found := make(map[string]*zip.File, len(webSDKAssets))
	for _, entry := range zr.File {
		normalized := strings.TrimPrefix(filepath.ToSlash(entry.Name), "./")
		for outputName, suffix := range webSDKAssets {
			if strings.EqualFold(normalized, suffix) || strings.HasSuffix(strings.ToLower(normalized), "/"+strings.ToLower(suffix)) {
				if !entry.FileInfo().IsDir() {
					found[outputName] = entry
				}
			}
		}
	}
	for name := range webSDKAssets {
		if found[name] == nil {
			return WebSDKInstallResult{}, fmt.Errorf("WebSDK archive is missing %s", name)
		}
	}

	staging := s.webSDKRoot + ".new"
	_ = os.RemoveAll(staging)
	if err := os.MkdirAll(staging, 0750); err != nil {
		return WebSDKInstallResult{}, err
	}
	defer os.RemoveAll(staging)

	var total uint64
	for name, entry := range found {
		total += entry.UncompressedSize64
		if total > uint64(maxWebSDKArchiveSize) {
			return WebSDKInstallResult{}, errors.New("WebSDK archive expands beyond safety limit")
		}
		src, err := entry.Open()
		if err != nil {
			return WebSDKInstallResult{}, err
		}
		dst := filepath.Join(staging, name)
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0640)
		if err != nil {
			_ = src.Close()
			return WebSDKInstallResult{}, err
		}
		_, copyErr := io.Copy(out, io.LimitReader(src, maxWebSDKArchiveSize+1))
		closeErr := out.Close()
		_ = src.Close()
		if copyErr != nil {
			return WebSDKInstallResult{}, copyErr
		}
		if closeErr != nil {
			return WebSDKInstallResult{}, closeErr
		}
	}

	backup := s.webSDKRoot + ".old"
	_ = os.RemoveAll(backup)
	if _, err := os.Stat(s.webSDKRoot); err == nil {
		if err := os.Rename(s.webSDKRoot, backup); err != nil {
			return WebSDKInstallResult{}, err
		}
	}
	if err := os.Rename(staging, s.webSDKRoot); err != nil {
		if _, statErr := os.Stat(backup); statErr == nil {
			_ = os.Rename(backup, s.webSDKRoot)
		}
		return WebSDKInstallResult{}, err
	}
	_ = os.RemoveAll(backup)

	status := s.webSDKStatus()
	return WebSDKInstallResult{
		Installed: status.Available,
		Version:   "3.3.1",
		Status:    status,
	}, nil
}

func (s *Service) WebSDKAsset(name string) (WebSDKAsset, error) {
	if _, ok := webSDKAssets[name]; !ok {
		return WebSDKAsset{}, os.ErrNotExist
	}
	status := s.webSDKStatus()
	if !status.Available {
		return WebSDKAsset{}, errors.New(status.Error)
	}
	path := filepath.Join(s.webSDKRoot, name)
	data, err := os.ReadFile(path)
	if err != nil {
		return WebSDKAsset{}, err
	}
	contentType := mime.TypeByExtension(filepath.Ext(name))
	if strings.HasSuffix(name, ".js") {
		contentType = "application/javascript; charset=utf-8"
	}
	if name == "HCWebSDKPlugin.exe" {
		contentType = "application/vnd.microsoft.portable-executable"
	}
	return WebSDKAsset{
		Name:        name,
		ContentType: contentType,
		Download:    name == "HCWebSDKPlugin.exe",
		Data:        data,
	}, nil
}
