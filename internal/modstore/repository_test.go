package modstore

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/DeadSoulf/home-ai-core/internal/modules"
)

func TestSignedRepositoryAndPackageVerification(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	trust := TrustStore{"official-2026": publicKey}

	payload := []byte("signed module package payload")
	digest := sha256.Sum256(payload)
	pkg := Package{
		URL:       "https://modules.example.invalid/storage/1.0.0/module.tar.zst",
		SHA256:    hex.EncodeToString(digest[:]),
		SizeBytes: int64(len(payload)),
		Signature: Signature{
			KeyID:     "official-2026",
			Algorithm: "ed25519",
			Value:     base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, digest[:])),
		},
	}

	index := RepositoryIndex{
		SchemaVersion: RepositorySchemaVersion,
		ID:            "official",
		GeneratedAt:   "2026-09-28T14:00:00Z",
		Releases: []Release{{
			Manifest: modules.Manifest{
				SchemaVersion: modules.ManifestSchemaVersion,
				ID:            "storage",
				Name:          "Storage",
				Version:       "1.0.0",
				Core:          ">=0.1.0 <1.0.0",
				Runtime: modules.RuntimeSpec{
					Type: "docker",
					Docker: modules.DockerSpec{
						Image: "ghcr.io/home-ai/storage@sha256:" + strings.Repeat("a", 64),
					},
				},
				Capabilities: modules.Capabilities{Requires: []string{"host.docker"}},
				Lifecycle:    []string{"install", "upgrade", "remove"},
			},
			Package: pkg,
		}},
	}

	raw, err := json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	indexSignature := Signature{
		KeyID:     "official-2026",
		Algorithm: "ed25519",
		Value:     base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, raw)),
	}

	verified, err := VerifyIndex(raw, indexSignature, trust)
	if err != nil {
		t.Fatalf("VerifyIndex() error = %v", err)
	}
	if verified.Releases[0].Manifest.ID != "storage" {
		t.Fatalf("unexpected release: %#v", verified.Releases[0])
	}
	if err := VerifyPackage(bytes.NewReader(payload), pkg, trust); err != nil {
		t.Fatalf("VerifyPackage() error = %v", err)
	}
}

func TestRepositoryRejectsTamperedIndex(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"schema_version":1,"id":"official","generated_at":"2026-09-28T14:00:00Z","releases":[]}`)
	signature := Signature{
		KeyID:     "official-2026",
		Algorithm: "ed25519",
		Value:     base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, raw)),
	}
	raw[len(raw)-2] = ' '

	if _, err := VerifyIndex(raw, signature, TrustStore{"official-2026": publicKey}); err == nil {
		t.Fatal("tampered repository index was accepted")
	}
}

func TestPackageRejectsTamperedPayload(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	original := []byte("original")
	digest := sha256.Sum256(original)
	pkg := Package{
		URL:       "https://modules.example.invalid/test/1.0.0/module.tar.zst",
		SHA256:    hex.EncodeToString(digest[:]),
		SizeBytes: int64(len(original)),
		Signature: Signature{
			KeyID:     "official-2026",
			Algorithm: "ed25519",
			Value:     base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, digest[:])),
		},
	}
	if err := VerifyPackage(bytes.NewReader([]byte("tampered")), pkg, TrustStore{"official-2026": publicKey}); err == nil {
		t.Fatal("tampered package was accepted")
	}
}
