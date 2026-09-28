package updater

import (
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"
)

type Manifest struct {
	SchemaVersion int            `json:"schema_version"`
	Product       string         `json:"product"`
	Version       string         `json:"version"`
	Architecture  string         `json:"architecture"`
	Files         []ManifestFile `json:"files"`
}

type ManifestFile struct {
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"size_bytes"`
}

func (m Manifest) Validate() error {
	if m.SchemaVersion != 1 {
		return errors.New("unsupported update manifest schema")
	}
	if m.Product != "home-ai-core" {
		return errors.New("unexpected update product")
	}
	if strings.TrimSpace(m.Version) == "" || strings.TrimSpace(m.Architecture) == "" {
		return errors.New("update manifest is missing version or architecture")
	}
	if len(m.Files) == 0 {
		return errors.New("update manifest has no files")
	}

	haveCore := false
	haveWeb := false
	seen := make(map[string]struct{}, len(m.Files))
	for _, file := range m.Files {
		clean := filepath.ToSlash(filepath.Clean(file.Path))
		if clean != file.Path || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
			return errors.New("update manifest contains unsafe path")
		}
		if len(file.SHA256) != 64 || strings.ToLower(file.SHA256) != file.SHA256 || file.SizeBytes < 0 {
			return errors.New("update manifest contains invalid file metadata")
		}
		if _, err := hex.DecodeString(file.SHA256); err != nil {
			return errors.New("update manifest contains invalid checksum")
		}
		if _, exists := seen[clean]; exists {
			return errors.New("update manifest contains duplicate file path")
		}
		seen[clean] = struct{}{}
		if clean == "bin/home-ai-core" {
			haveCore = true
		}
		if strings.HasPrefix(clean, "web/") {
			haveWeb = true
		}
	}
	if !haveCore || !haveWeb {
		return errors.New("update manifest is missing core binary or web files")
	}
	return nil
}
