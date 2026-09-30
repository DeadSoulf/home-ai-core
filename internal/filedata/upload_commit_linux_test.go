//go:build linux

package filedata

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallUploadPartByLinkCleanupFailureStillCommits(t *testing.T) {
	root := t.TempDir()
	staging := filepath.Join(root, "staging")
	if err := os.Mkdir(staging, 0o750); err != nil {
		t.Fatal(err)
	}
	part := filepath.Join(staging, "part")
	payload := []byte("committed data")
	if err := os.WriteFile(part, payload, 0o640); err != nil {
		t.Fatal(err)
	}
	probe := filepath.Join(staging, "probe")
	if err := os.WriteFile(probe, nil, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(staging, 0o550); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(staging, 0o750) })
	if err := os.Remove(probe); !errors.Is(err, os.ErrPermission) {
		t.Skipf("filesystem permissions cannot force source cleanup failure: %v", err)
	}
	target := filepath.Join(root, "target")
	if err := installUploadPartByLink(part, target); err != nil {
		t.Fatalf("committed destination reported as failure: %v", err)
	}
	for _, path := range []string{part, target} {
		data, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(data, payload) {
			t.Fatalf("data at %s = %q, error = %v", path, data, err)
		}
	}
	if err := os.Chmod(staging, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(part); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil || !bytes.Equal(data, payload) {
		t.Fatalf("source cleanup removed committed destination: %q, %v", data, err)
	}
}
