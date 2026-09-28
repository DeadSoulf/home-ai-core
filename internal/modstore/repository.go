package modstore

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/modules"
)

const RepositorySchemaVersion = 1

var identifierPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[.-][a-z0-9]+)*$`)

type Signature struct {
	KeyID     string `json:"key_id"`
	Algorithm string `json:"algorithm"`
	Value     string `json:"value"`
}

type Package struct {
	URL       string    `json:"url"`
	SHA256    string    `json:"sha256"`
	SizeBytes int64     `json:"size_bytes"`
	Signature Signature `json:"signature"`
}

type Release struct {
	Manifest modules.Manifest `json:"manifest"`
	Package  Package          `json:"package"`
}

type RepositoryIndex struct {
	SchemaVersion int       `json:"schema_version"`
	ID            string    `json:"id"`
	GeneratedAt   string    `json:"generated_at"`
	Releases      []Release `json:"releases"`
}

type TrustStore map[string]ed25519.PublicKey

func VerifyIndex(raw []byte, signature Signature, trust TrustStore) (RepositoryIndex, error) {
	if err := verifySignature(raw, signature, trust); err != nil {
		return RepositoryIndex{}, fmt.Errorf("verify repository signature: %w", err)
	}

	var index RepositoryIndex
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&index); err != nil {
		return RepositoryIndex{}, fmt.Errorf("decode repository index: %w", err)
	}
	if decoder.More() {
		return RepositoryIndex{}, errors.New("repository index contains trailing JSON values")
	}
	if err := ValidateIndex(index); err != nil {
		return RepositoryIndex{}, err
	}
	return index, nil
}

func ValidateIndex(index RepositoryIndex) error {
	if index.SchemaVersion != RepositorySchemaVersion {
		return fmt.Errorf("unsupported repository schema version %d", index.SchemaVersion)
	}
	if !identifierPattern.MatchString(index.ID) {
		return errors.New("repository id is invalid")
	}
	if _, err := time.Parse(time.RFC3339, index.GeneratedAt); err != nil {
		return errors.New("repository generated_at must be RFC3339")
	}
	if len(index.Releases) == 0 {
		return errors.New("repository has no releases")
	}

	seen := map[string]struct{}{}
	for _, release := range index.Releases {
		if err := modules.ValidateManifest(release.Manifest); err != nil {
			return fmt.Errorf("release %q manifest: %w", release.Manifest.ID, err)
		}
		key := release.Manifest.ID + "@" + release.Manifest.Version
		if _, exists := seen[key]; exists {
			return fmt.Errorf("duplicate release %q", key)
		}
		seen[key] = struct{}{}

		if err := validatePackage(release.Package); err != nil {
			return fmt.Errorf("release %q package: %w", key, err)
		}
	}
	return nil
}

func VerifyPackage(reader io.Reader, pkg Package, trust TrustStore) error {
	if err := validatePackage(pkg); err != nil {
		return err
	}

	hash := sha256.New()
	written, err := io.Copy(hash, reader)
	if err != nil {
		return fmt.Errorf("hash module package: %w", err)
	}
	if written != pkg.SizeBytes {
		return fmt.Errorf("package size mismatch: got %d, want %d", written, pkg.SizeBytes)
	}

	digest := hash.Sum(nil)
	expected, _ := hex.DecodeString(pkg.SHA256)
	if !equalBytes(digest, expected) {
		return errors.New("package sha256 mismatch")
	}
	if err := verifySignature(digest, pkg.Signature, trust); err != nil {
		return fmt.Errorf("verify package signature: %w", err)
	}
	return nil
}

func validatePackage(pkg Package) error {
	parsed, err := url.Parse(pkg.URL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return errors.New("package url must use absolute https")
	}
	if pkg.SizeBytes <= 0 {
		return errors.New("package size_bytes must be positive")
	}
	if len(pkg.SHA256) != sha256.Size*2 {
		return errors.New("package sha256 must be a 64-character hex digest")
	}
	if _, err := hex.DecodeString(pkg.SHA256); err != nil || strings.ToLower(pkg.SHA256) != pkg.SHA256 {
		return errors.New("package sha256 must be lowercase hexadecimal")
	}
	if pkg.Signature.KeyID == "" || pkg.Signature.Value == "" {
		return errors.New("package signature is required")
	}
	if pkg.Signature.Algorithm != "ed25519" {
		return errors.New("package signature algorithm must be ed25519")
	}
	return nil
}

func verifySignature(message []byte, signature Signature, trust TrustStore) error {
	if signature.Algorithm != "ed25519" {
		return errors.New("signature algorithm must be ed25519")
	}
	key, ok := trust[signature.KeyID]
	if !ok || len(key) != ed25519.PublicKeySize {
		return fmt.Errorf("untrusted signing key %q", signature.KeyID)
	}
	raw, err := base64.StdEncoding.DecodeString(signature.Value)
	if err != nil || len(raw) != ed25519.SignatureSize {
		return errors.New("invalid ed25519 signature encoding")
	}
	if !ed25519.Verify(key, message, raw) {
		return errors.New("signature verification failed")
	}
	return nil
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := range a {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}
