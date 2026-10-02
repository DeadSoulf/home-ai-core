package nvr

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileCredentialStoreEncryptsAndResolvesCameraCredential(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileCredentialStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := store.PutCameraCredential(context.Background(), "cam_test", CameraCredential{
		Username: "camera-user",
		Password: "super-secret-password",
	})
	if err != nil {
		t.Fatal(err)
	}
	if ref == "" {
		t.Fatal("empty secret reference")
	}

	secretPath := filepath.Join(dir, credentialDirName, string(ref)+".bin")
	raw, err := os.ReadFile(secretPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "camera-user") || strings.Contains(string(raw), "super-secret-password") {
		t.Fatal("credential file contains plaintext credential")
	}
	info, err := os.Stat(secretPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("secret permissions = %o, want 600", info.Mode().Perm())
	}

	credential, err := store.ResolveCameraCredential(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if credential.Username != "camera-user" || credential.Password != "super-secret-password" {
		t.Fatalf("credential = %#v", credential)
	}

	if err := store.DeleteCameraCredential(context.Background(), ref); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(secretPath); !os.IsNotExist(err) {
		t.Fatalf("secret still exists after delete: %v", err)
	}
}
