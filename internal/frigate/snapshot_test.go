package frigate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigurationSnapshotIncludesSQLiteSidecars(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "config")
	dst := filepath.Join(root, "backup")
	if err := os.Mkdir(src, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"config.yml", "frigate.db", "frigate.db-wal", "frigate.db-shm"} {
		if err := os.WriteFile(filepath.Join(src, name), []byte("original:"+name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(src, "empty"), 0700); err != nil {
		t.Fatal(err)
	}
	snapshot, err := CopyConfig(src, dst, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot) != 5 {
		t.Fatalf("missing configuration data: %#v", snapshot)
	}
	if err := VerifyConfig(dst, snapshot, 1<<20); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "frigate.db"), []byte("new schema"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyConfig(dst, snapshot, 1<<20); err != nil {
		t.Fatal("backup changed with source", err)
	}
	if err := os.WriteFile(filepath.Join(dst, "frigate.db"), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyConfig(dst, snapshot, 1<<20); err == nil {
		t.Fatal("accepted a corrupt database backup")
	}
}
func TestConfigurationSnapshotRejectsLinksAndLimits(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "config")
	if err := os.Mkdir(src, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "config.yml"), []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := CopyConfig(src, filepath.Join(root, "too-small"), 2); err == nil {
		t.Fatal("ignored size bound")
	}
	if _, err := CopyConfig(src, filepath.Join(src, "nested"), 100); err == nil {
		t.Fatal("accepted nested snapshot")
	}
	if err := os.Symlink("/etc/passwd", filepath.Join(src, "escape")); err != nil {
		t.Skip(err)
	}
	if _, err := CopyConfig(src, filepath.Join(root, "linked"), 1<<20); err == nil {
		t.Fatal("followed a symlink")
	}
}
func TestConfigurationSnapshotDoesNotOverwriteExistingBackup(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source")
	dst := filepath.Join(root, "backup")
	_ = os.Mkdir(src, 0700)
	_ = os.Mkdir(dst, 0700)
	_ = os.WriteFile(filepath.Join(dst, "keep"), []byte("keep"), 0600)
	if _, err := CopyConfig(src, dst, 1<<20); err == nil {
		t.Fatal("overwrote existing backup")
	}
	if b, err := os.ReadFile(filepath.Join(dst, "keep")); err != nil || string(b) != "keep" {
		t.Fatal("backup lost")
	}
}
