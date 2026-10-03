package cameras

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const maxSDKArchiveSize int64 = 256 << 20

type InstallResult struct {
	Installed bool   `json:"installed"`
	Path      string `json:"path"`
	Status    Status `json:"status"`
}

func (s *Service) InstallRuntime(reader io.Reader, size int64) (InstallResult, error) {
	if s == nil || s.runtime == nil {
		return InstallResult{}, ErrSDKUnavailable
	}
	if s.runtimeRoot == "" {
		return InstallResult{}, errors.New("HCNetSDK runtime directory is not configured")
	}
	if size <= 0 || size > maxSDKArchiveSize {
		return InstallResult{}, fmt.Errorf("HCNetSDK archive must be between 1 byte and %d bytes", maxSDKArchiveSize)
	}

	tmp, err := os.CreateTemp("", "home-ai-hcnetsdk-*.zip")
	if err != nil { return InstallResult{}, err }
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err = io.CopyN(tmp, reader, size); err != nil {
		tmp.Close()
		return InstallResult{}, fmt.Errorf("read HCNetSDK archive: %w", err)
	}
	if err = tmp.Close(); err != nil { return InstallResult{}, err }

	zr, err := zip.OpenReader(tmpName)
	if err != nil { return InstallResult{}, fmt.Errorf("open HCNetSDK archive: %w", err) }
	defer zr.Close()

	prefix := ""
	for _, f := range zr.File {
		name := filepath.ToSlash(f.Name)
		if strings.HasSuffix(name, "/lib/libhcnetsdk.so") {
			prefix = strings.TrimSuffix(name, "lib/libhcnetsdk.so")
			break
		}
	}
	if prefix == "" {
		return InstallResult{}, errors.New("archive does not contain Linux amd64 HCNetSDK lib/libhcnetsdk.so")
	}

	staging := s.runtimeRoot + ".new"
	_ = os.RemoveAll(staging)
	if err := os.MkdirAll(filepath.Join(staging, "lib"), 0750); err != nil { return InstallResult{}, err }
	defer os.RemoveAll(staging)

	files := 0
	var total uint64
	for _, f := range zr.File {
		name := filepath.ToSlash(f.Name)
		wantPrefix := prefix + "lib/"
		if !strings.HasPrefix(name, wantPrefix) { continue }
		rel := strings.TrimPrefix(name, wantPrefix)
		if rel == "" || f.FileInfo().IsDir() { continue }
		clean := filepath.Clean(filepath.FromSlash(rel))
		if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return InstallResult{}, errors.New("unsafe path in HCNetSDK archive")
		}
		total += f.UncompressedSize64
		if total > uint64(maxSDKArchiveSize) { return InstallResult{}, errors.New("HCNetSDK archive expands beyond safety limit") }
		dst := filepath.Join(staging, "lib", clean)
		if err := os.MkdirAll(filepath.Dir(dst), 0750); err != nil { return InstallResult{}, err }
		src, err := f.Open()
		if err != nil { return InstallResult{}, err }
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0640)
		if err != nil { src.Close(); return InstallResult{}, err }
		_, copyErr := io.Copy(out, src)
		closeErr := out.Close()
		src.Close()
		if copyErr != nil { return InstallResult{}, copyErr }
		if closeErr != nil { return InstallResult{}, closeErr }
		files++
	}
	if files == 0 {
		return InstallResult{}, errors.New("HCNetSDK archive contains no runtime files")
	}
	if _, err := os.Stat(filepath.Join(staging, "lib", "libhcnetsdk.so")); err != nil {
		return InstallResult{}, errors.New("HCNetSDK runtime library was not extracted")
	}

	backup := s.runtimeRoot + ".old"
	_ = os.RemoveAll(backup)
	if _, err := os.Stat(s.runtimeRoot); err == nil {
		if err := os.Rename(s.runtimeRoot, backup); err != nil { return InstallResult{}, err }
	}
	if err := os.Rename(staging, s.runtimeRoot); err != nil {
		if _, statErr := os.Stat(backup); statErr == nil { _ = os.Rename(backup, s.runtimeRoot) }
		return InstallResult{}, err
	}
	_ = os.RemoveAll(backup)
	_ = os.Setenv("HOME_AI_HCNETSDK_DIR", s.runtimeRoot)
	status := s.Refresh()
	return InstallResult{Installed: status.SDK.Available && status.SDK.Initialized, Path: s.runtimeRoot, Status: status}, nil
}
