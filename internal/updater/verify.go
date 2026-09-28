package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func VerifyPreparedBundle(root, version string) (Manifest, error) {
	data, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		return Manifest{}, fmt.Errorf("read update manifest: %w", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode update manifest: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}
	if manifest.Version != version {
		return Manifest{}, fmt.Errorf("manifest version %q does not match %q", manifest.Version, version)
	}
	if manifest.Architecture != runtime.GOARCH {
		return Manifest{}, fmt.Errorf("manifest architecture %q does not match host %q", manifest.Architecture, runtime.GOARCH)
	}

	expected := make(map[string]ManifestFile, len(manifest.Files))
	for _, entry := range manifest.Files {
		expected[entry.Path] = entry
		path := filepath.Join(root, filepath.FromSlash(entry.Path))
		info, err := os.Lstat(path)
		if err != nil {
			return Manifest{}, fmt.Errorf("stat update file %q: %w", entry.Path, err)
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return Manifest{}, fmt.Errorf("update file %q is not a regular file", entry.Path)
		}
		if info.Size() != entry.SizeBytes {
			return Manifest{}, fmt.Errorf("update file %q size mismatch", entry.Path)
		}
		file, err := os.Open(path)
		if err != nil {
			return Manifest{}, err
		}
		hash := sha256.New()
		_, copyErr := io.Copy(hash, file)
		closeErr := file.Close()
		if copyErr != nil {
			return Manifest{}, copyErr
		}
		if closeErr != nil {
			return Manifest{}, closeErr
		}
		if hex.EncodeToString(hash.Sum(nil)) != entry.SHA256 {
			return Manifest{}, fmt.Errorf("update file %q checksum mismatch", entry.Path)
		}
	}

	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "manifest.json" {
			return nil
		}
		if strings.HasPrefix(rel, "../") || strings.HasPrefix(rel, "/") {
			return fmt.Errorf("unsafe prepared update path %q", rel)
		}
		if _, ok := expected[rel]; !ok {
			return fmt.Errorf("unexpected prepared update file %q", rel)
		}
		return nil
	})
	if err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}
