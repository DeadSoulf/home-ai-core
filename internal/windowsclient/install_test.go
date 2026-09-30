package windowsclient

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallExecutableAtomicCopyAndUpdate(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.exe")
	destination := filepath.Join(root, "installed", "client.exe")
	if err := os.WriteFile(source, []byte("version-one"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := installExecutable(source, destination); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(destination)
	if err != nil || string(data) != "version-one" {
		t.Fatalf("installed data = %q, err %v", data, err)
	}
	if err := os.WriteFile(source, []byte("version-two"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := installExecutable(source, destination); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(destination)
	if err != nil || string(data) != "version-two" {
		t.Fatalf("updated data = %q, err %v", data, err)
	}
}

func TestInstallExecutableRejectsSymlinkSource(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.exe")
	if err := os.WriteFile(source, []byte("data"), 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link.exe")
	if err := os.Symlink(source, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := installExecutable(link, filepath.Join(root, "installed.exe")); err == nil {
		t.Fatal("accepted symlink source")
	}
}
