package updater

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func TestExtractAndVerifyBundleAcceptsValidArchive(t *testing.T) {
	files := map[string][]byte{
		"bin/home-ai-core": []byte("core-binary"),
		"web/index.html":   []byte("<html>home-ai</html>"),
	}
	archive := writeTestUpdateArchive(t, "0.1.109-dev", runtime.GOARCH, files, files, nil)
	target := filepath.Join(t.TempDir(), "prepared")

	if err := extractAndVerifyBundle(archive, target, "0.1.109-dev", runtime.GOARCH); err != nil {
		t.Fatalf("extractAndVerifyBundle() error = %v", err)
	}
}

func TestExtractAndVerifyBundleRejectsChecksumMismatch(t *testing.T) {
	expected := map[string][]byte{
		"bin/home-ai-core": []byte("expected-core"),
		"web/index.html":   []byte("<html>home-ai</html>"),
	}
	actual := map[string][]byte{
		"bin/home-ai-core": []byte("tampered-core"),
		"web/index.html":   expected["web/index.html"],
	}
	archive := writeTestUpdateArchive(t, "0.1.109-dev", runtime.GOARCH, expected, actual, nil)
	target := filepath.Join(t.TempDir(), "prepared")

	err := extractAndVerifyBundle(archive, target, "0.1.109-dev", runtime.GOARCH)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("extractAndVerifyBundle() error = %v, want checksum mismatch", err)
	}
}

func TestExtractAndVerifyBundleRejectsUnexpectedFile(t *testing.T) {
	files := map[string][]byte{
		"bin/home-ai-core": []byte("core-binary"),
		"web/index.html":   []byte("<html>home-ai</html>"),
	}
	archive := writeTestUpdateArchive(t, "0.1.109-dev", runtime.GOARCH, files, files, map[string][]byte{
		"unexpected.txt": []byte("must not be accepted"),
	})
	target := filepath.Join(t.TempDir(), "prepared")

	err := extractAndVerifyBundle(archive, target, "0.1.109-dev", runtime.GOARCH)
	if err == nil || !strings.Contains(err.Error(), "unexpected file") {
		t.Fatalf("extractAndVerifyBundle() error = %v, want unexpected file rejection", err)
	}
}

func TestExtractAndVerifyBundleRejectsPathTraversal(t *testing.T) {
	files := map[string][]byte{
		"bin/home-ai-core": []byte("core-binary"),
		"web/index.html":   []byte("<html>home-ai</html>"),
	}
	archive := writeTestUpdateArchive(t, "0.1.109-dev", runtime.GOARCH, files, files, map[string][]byte{
		"../escape": []byte("must not escape"),
	})
	target := filepath.Join(t.TempDir(), "prepared")

	err := extractAndVerifyBundle(archive, target, "0.1.109-dev", runtime.GOARCH)
	if err == nil || !strings.Contains(err.Error(), "unsafe path") {
		t.Fatalf("extractAndVerifyBundle() error = %v, want unsafe path rejection", err)
	}
}

func writeTestUpdateArchive(t *testing.T, version, architecture string, expected, actual, extra map[string][]byte) string {
	t.Helper()

	manifest := Manifest{
		SchemaVersion: 1,
		Product:       "home-ai-core",
		Version:       version,
		Architecture:  architecture,
	}
	paths := make([]string, 0, len(expected))
	for path := range expected {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		payload := expected[path]
		sum := sha256.Sum256(payload)
		manifest.Files = append(manifest.Files, ManifestFile{
			Path:      path,
			SHA256:    hex.EncodeToString(sum[:]),
			SizeBytes: int64(len(payload)),
		})
	}
	manifestData, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}

	archivePath := filepath.Join(t.TempDir(), "update.tar.gz")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(file)
	tw := tar.NewWriter(gz)

	writeEntry := func(name string, payload []byte) {
		t.Helper()
		if err := tw.WriteHeader(&tar.Header{
			Name: name,
			Mode: 0o600,
			Size: int64(len(payload)),
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(payload); err != nil {
			t.Fatal(err)
		}
	}

	writeEntry("manifest.json", manifestData)
	for _, path := range paths {
		payload, ok := actual[path]
		if !ok {
			payload = expected[path]
		}
		writeEntry(path, payload)
	}
	extraPaths := make([]string, 0, len(extra))
	for path := range extra {
		extraPaths = append(extraPaths, path)
	}
	sort.Strings(extraPaths)
	for _, path := range extraPaths {
		writeEntry(path, extra[path])
	}

	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return archivePath
}
