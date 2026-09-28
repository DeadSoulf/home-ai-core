package identity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrCreateIsStable(t *testing.T) {
	dir := t.TempDir()

	first, err := LoadOrCreate(dir)
	if err != nil {
		t.Fatalf("first LoadOrCreate() error = %v", err)
	}
	second, err := LoadOrCreate(dir)
	if err != nil {
		t.Fatalf("second LoadOrCreate() error = %v", err)
	}

	if first != second {
		t.Fatalf("node id changed: %q != %q", first, second)
	}
	if !validUUID(first) {
		t.Fatalf("node id is not a valid UUID: %q", first)
	}

	info, err := os.Stat(filepath.Join(dir, "identity", nodeIDFile))
	if err != nil {
		t.Fatalf("stat node id: %v", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("node id permissions are too broad: %o", info.Mode().Perm())
	}
}
