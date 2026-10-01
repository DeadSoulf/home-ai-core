package updater

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestDownloadArchiveRetriesTransientGatewayErrors(t *testing.T) {
	oldDelays := downloadRetryDelays
	downloadRetryDelays = []time.Duration{0, 0, 0}
	t.Cleanup(func() { downloadRetryDelays = oldDelays })

	payload := []byte("verified update bundle payload")
	sum := sha256.Sum256(payload)
	expectedHash := hex.EncodeToString(sum[:])
	var requests atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) < 3 {
			http.Error(w, "temporary upstream failure", http.StatusGatewayTimeout)
			return
		}
		w.Header().Set("Content-Length", "30")
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	service := &Service{
		currentVersion: "0.1.93-dev",
		bundleClient:   &http.Client{Timeout: time.Second},
	}
	target := filepath.Join(t.TempDir(), "update.tar.gz")
	if err := service.downloadArchive(t.Context(), server.URL, target, int64(len(payload)), expectedHash); err != nil {
		t.Fatalf("downloadArchive() error = %v", err)
	}
	if got := requests.Load(); got != 3 {
		t.Fatalf("requests = %d, want 3", got)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(payload) {
		t.Fatalf("downloaded payload = %q, want %q", data, payload)
	}
}

func TestDownloadArchiveDoesNotRetryPermanentHTTPError(t *testing.T) {
	oldDelays := downloadRetryDelays
	downloadRetryDelays = []time.Duration{0, 0, 0}
	t.Cleanup(func() { downloadRetryDelays = oldDelays })

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.NotFound(w, r)
	}))
	defer server.Close()

	service := &Service{
		currentVersion: "0.1.93-dev",
		bundleClient:   &http.Client{Timeout: time.Second},
	}
	err := service.downloadArchive(t.Context(), server.URL, filepath.Join(t.TempDir(), "update.tar.gz"), 1, "00")
	if err == nil {
		t.Fatal("downloadArchive() error = nil, want failure")
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("requests = %d, want 1", got)
	}
}

func TestRequiresSignedUpdate(t *testing.T) {
	if requiresSignedUpdate("0.2.0-dev") {
		t.Fatal("development release unexpectedly requires a signature")
	}
	if !requiresSignedUpdate("0.2.0") {
		t.Fatal("stable release must require a signature")
	}
	if !requiresSignedUpdate("0.2.0-rc.1") {
		t.Fatal("release candidate must require a signature")
	}
}

func TestVerifyChecksumSignature(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	checksum := []byte("0123456789abcdef  home-ai-core-update_0.2.0_amd64.tar.gz\n")
	signature := ed25519.Sign(privateKey, checksum)
	der, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	publicPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
	signatureText := []byte(base64.StdEncoding.EncodeToString(signature) + "\n")

	if err := verifyChecksumSignature(checksum, signatureText, publicPEM); err != nil {
		t.Fatalf("verifyChecksumSignature() error = %v", err)
	}

	tampered := append([]byte(nil), checksum...)
	tampered[0] = 'f'
	if err := verifyChecksumSignature(tampered, signatureText, publicPEM); err == nil {
		t.Fatal("tampered checksum unexpectedly verified")
	}
}
