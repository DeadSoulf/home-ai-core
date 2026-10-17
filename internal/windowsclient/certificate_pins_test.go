package windowsclient

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCertificatePinStorePerServer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pins.json")
	a := "https://homeaicore.ru:82"
	b := "https://another.example:82"
	first := strings.Repeat("A", 64)
	second := strings.Repeat("b", 64)
	if err := saveServerPinAt(path, a, first); err != nil {
		t.Fatal(err)
	}
	if err := saveServerPinAt(path, b, second); err != nil {
		t.Fatal(err)
	}
	got, err := loadServerPinAt(path, "https://HOMEAICORE.RU:82/")
	if err != nil || got != strings.ToLower(first) {
		t.Fatalf("saved origin pin = %q, %v", got, err)
	}
	other, err := loadServerPinAt(path, b)
	if err != nil || other != second {
		t.Fatalf("other server pin = %q, %v", other, err)
	}
	if err := saveServerPinAt(path, a, ""); err != nil {
		t.Fatal(err)
	}
	got, err = loadServerPinAt(path, a)
	if err != nil || got != "" {
		t.Fatalf("removed pin = %q, %v", got, err)
	}
	other, err = loadServerPinAt(path, b)
	if err != nil || other != second {
		t.Fatalf("removing one origin changed another: %q, %v", other, err)
	}
}

func TestCertificatePinStoreRejectsInvalidInput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pins.json")
	for _, input := range []string{
		"http://127.0.0.1:82", "https://user:pass@host:82",
		"https://host:82?password=x", "https://host:0", "https://host:65536",
	} {
		if err := saveServerPinAt(path, input, strings.Repeat("a", 64)); err == nil {
			t.Fatalf("accepted unsafe pin origin %q", input)
		}
	}
	for _, pin := range []string{"1234", strings.Repeat("f", 63), strings.Repeat("z", 64)} {
		if err := saveServerPinAt(path, "https://localhost:82", pin); err == nil {
			t.Fatalf("accepted invalid pin %q", pin)
		}
	}
}

func TestPinnedTLSAuthenticatesExactCertificateWithoutSystemCA(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	leaf, err := x509.ParseCertificate(server.TLS.Certificates[0].Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	sha := sha256.Sum256(leaf.Raw)
	fingerprint := hex.EncodeToString(sha[:])

	probed, err := ProbeServerCertificate(context.Background(), server.URL)
	if err != nil || probed.SHA256 != fingerprint || requests.Load() != 0 {
		t.Fatalf("unauthenticated probe = %+v, %v, requests=%d", probed, err, requests.Load())
	}

	// Simulate HTTPS to a public name that is missing from the self-signed
	// local certificate SAN. A verified pin authenticates the exact leaf.
	cfg, err := pinnedTLSConfig("homeaicore.ru", fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.InsecureSkipVerify || cfg.VerifyConnection == nil {
		t.Fatal("pinned TLS must enforce its explicit VerifyConnection callback")
	}
	var dialer net.Dialer
	transport := &http.Transport{
		TLSClientConfig: cfg,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "tcp", server.Listener.Addr().String())
		},
	}
	client := &http.Client{Timeout: 5 * time.Second, Transport: transport}
	response, err := client.Get("https://homeaicore.ru:82/health")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if requests.Load() != 1 {
		t.Fatal("verified pinned TLS did not send request")
	}

	badConfig, err := pinnedTLSConfig("homeaicore.ru", strings.Repeat("1", 64))
	if err != nil {
		t.Fatal(err)
	}
	badTransport := transport.Clone()
	badTransport.TLSClientConfig = badConfig
	badClient := &http.Client{Timeout: 5 * time.Second, Transport: badTransport}
	_, err = badClient.Get("https://homeaicore.ru:82/health")
	if err == nil || !strings.Contains(err.Error(), "certificate has changed") {
		t.Fatalf("changed certificate accepted: %v", err)
	}
	if requests.Load() != 1 {
		t.Fatalf("sent request over untrusted TLS: %d", requests.Load())
	}

	plainConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	standardTransport := transport.Clone()
	standardTransport.TLSClientConfig = plainConfig
	standardClient := &http.Client{Timeout: 5 * time.Second, Transport: standardTransport}
	_, err = standardClient.Get("https://homeaicore.ru:82/health")
	var unknownAuthority x509.UnknownAuthorityError
	if !errors.As(err, &unknownAuthority) {
		t.Fatalf("ordinary strict TLS unexpectedly succeeded: %v", err)
	}
}
