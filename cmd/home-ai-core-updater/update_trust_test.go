package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/DeadSoulf/home-ai-core/internal/updater"
)

func signedFixture(t *testing.T, incoming string) ([]byte, string) {
	t.Helper()
	version := "0.1.173-dev"
	name := fmt.Sprintf("home-ai-core-update_%s_%s.tar.gz", version, runtime.GOARCH)
	files := map[string][]byte{"bin/home-ai-core": []byte("trusted core"), "web/index.html": []byte("trusted web"), "helper/home-ai-core-updater": []byte("trusted helper")}
	manifest := updater.Manifest{SchemaVersion: 1, Product: "home-ai-core", Version: version, Architecture: runtime.GOARCH}
	for path, data := range files {
		sum := sha256.Sum256(data)
		manifest.Files = append(manifest.Files, updater.ManifestFile{Path: path, SizeBytes: int64(len(data)), SHA256: hex.EncodeToString(sum[:])})
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	files["manifest.json"] = raw
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for path, data := range files {
		if err := tw.WriteHeader(&tar.Header{Name: path, Mode: 0o600, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(buf.Bytes())
	checksum := []byte(fmt.Sprintf("%x  %s\n", sum, name))
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signature := []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(priv, checksum)))
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	for suffix, data := range map[string][]byte{"": buf.Bytes(), ".sha256": checksum, ".sha256.sig": signature} {
		if err := os.WriteFile(filepath.Join(incoming, name+suffix), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}), name
}

func TestSignedStageIsIndependentOfCoreWritablePreparedFiles(t *testing.T) {
	incoming := t.TempDir()
	private := t.TempDir()
	key, name := signedFixture(t, incoming)
	// An attacker-controlled legacy 'prepared' directory is never consulted.
	evil := filepath.Join(incoming, "prepared-0.1.173-dev", "helper")
	os.MkdirAll(evil, 0o700)
	os.WriteFile(filepath.Join(evil, "home-ai-core-updater"), []byte("evil"), 0o600)
	prepared, cleanup, err := stageSignedUpdateAt(context.Background(), incoming, private, key, "0.1.173-dev", runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if err := os.WriteFile(filepath.Join(incoming, name), []byte("mutated after verification"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(prepared, "helper", "home-ai-core-updater"))
	if err != nil || string(got) != "trusted helper" {
		t.Fatalf("immutable snapshot violated %q %v", got, err)
	}
}

func TestSignedStageRejectsTamperingMissingSignatureAndWrongKey(t *testing.T) {
	for _, kind := range []string{"archive", "signature", "missing", "key", "symlink", "name"} {
		t.Run(kind, func(t *testing.T) {
			incoming := t.TempDir()
			private := t.TempDir()
			key, name := signedFixture(t, incoming)
			switch kind {
			case "archive":
				os.WriteFile(filepath.Join(incoming, name), []byte("evil"), 0o600)
			case "signature":
				os.WriteFile(filepath.Join(incoming, name+".sha256.sig"), []byte("invalid"), 0o600)
			case "missing":
				os.Remove(filepath.Join(incoming, name+".sha256.sig"))
			case "key":
				key, _ = signedFixture(t, t.TempDir())
			case "symlink":
				src := filepath.Join(incoming, name)
				os.Rename(src, src+".original")
				os.Symlink(src+".original", src)
			case "name":
				name2 := "home-ai-core-update_0.1.174-dev_" + runtime.GOARCH + ".tar.gz"
				for _, suffix := range []string{"", ".sha256", ".sha256.sig"} {
					os.Rename(filepath.Join(incoming, name+suffix), filepath.Join(incoming, name2+suffix))
				}
			}
			version := "0.1.173-dev"
			if kind == "name" {
				version = "0.1.174-dev"
			}
			if _, cleanup, err := stageSignedUpdateAt(context.Background(), incoming, private, key, version, runtime.GOARCH); err == nil {
				if cleanup != nil {
					cleanup()
				}
				t.Fatal("untrusted update accepted")
			}
			entries, _ := os.ReadDir(private)
			if len(entries) != 0 {
				t.Fatal("failed verification left staged payloads")
			}
		})
	}
}

func TestUntrustedResultCannotFollowSymlink(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(t.TempDir(), "victim")
	os.WriteFile(victim, []byte("preserve"), 0o600)
	if err := os.Symlink(victim, filepath.Join(dir, "install-result.json")); err != nil {
		t.Fatal(err)
	}
	if err := writeUntrustedResult(dir, os.Getuid(), os.Getgid(), []byte("{}\n")); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(victim)
	if string(data) != "preserve" {
		t.Fatal("root result write followed a symlink")
	}
	alias := filepath.Join(t.TempDir(), "alias")
	os.Symlink(dir, alias)
	if err := writeUntrustedResult(alias, os.Getuid(), os.Getgid(), []byte("bad")); err == nil {
		t.Fatal("result directory symlink accepted")
	}
}

func TestStatePreparationRejectsSymlinkChild(t *testing.T) {
	dir := t.TempDir()
	victim := t.TempDir()
	os.Symlink(victim, filepath.Join(dir, "update"))
	parent, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	if child, err := prepareDirectoryAt(parent, "update", os.Getuid(), os.Getgid(), 0o700); err == nil {
		child.Close()
		t.Fatal("state preparation followed symlink")
	}
}

func TestTrustAnchorRejectsUnprivilegedOrWritablePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "key")
	os.WriteFile(path, []byte("not a trusted key"), 0o666)
	if err := requireRootOwnedPath(path); err == nil {
		t.Fatal("writable key path accepted")
	}
	if filepath.Dir(trustedUpdateRoot) == updateRoot {
		t.Fatal("trusted backups are below a Core-writable directory")
	}
}
