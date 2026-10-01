package updater

import (
	"crypto/sha256"
	"encoding/hex"
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
