package frigate

import (
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"testing"
)

func TestSnapshotPreservesModesUnderRestrictiveHelperUmask(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source")
	_ = os.Mkdir(src, 0700)
	nested := filepath.Join(src, "models")
	_ = os.Mkdir(nested, 0755)
	_ = os.Chmod(nested, 0755)
	file := filepath.Join(nested, "model.bin")
	_ = os.WriteFile(file, []byte("model"), 0644)
	_ = os.Chmod(file, 0644)
	previous := unix.Umask(0077)
	defer unix.Umask(previous)
	snapshot, err := CopyConfig(src, filepath.Join(root, "backup"), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot["models/model.bin"].Mode != 0644 || snapshot["models"].Mode != 0755 {
		t.Fatal("snapshot permission metadata changed")
	}
	if err := VerifyConfig(filepath.Join(root, "backup"), snapshot, 1<<20); err != nil {
		t.Fatal(err)
	}
}
