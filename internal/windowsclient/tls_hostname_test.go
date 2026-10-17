package windowsclient

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// Trusting a leaf certificate must never suppress matching the TLS server name.
// The test connects to an in-process HTTPS server through a custom dialer, so
// it makes no request to the public hostname used in the certificate check.
func TestTrustedLocalCertificateRejectsUnlistedPublicHostname(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cert, err := x509.ParseCertificate(server.TLS.Certificates[0].Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	ca := filepath.Join(t.TempDir(), "verified-server.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME_AI_CA_FILE", ca)

	client, err := New("https://homeaicore.ru:82")
	if err != nil {
		t.Fatal(err)
	}
	transport := client.HTTPClient.Transport.(*http.Transport)
	var dialer net.Dialer
	transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
		return dialer.DialContext(ctx, "tcp", server.Listener.Addr().String())
	}
	response, err := client.HTTPClient.Get(client.BaseURL.String() + "/health")
	if response != nil {
		response.Body.Close()
	}
	if err == nil {
		t.Fatal("trusted leaf certificate incorrectly disabled hostname validation")
	}
	var nameMismatch x509.HostnameError
	if !errors.As(err, &nameMismatch) {
		t.Fatalf("expected a hostname verification error, got %T: %v", err, err)
	}
}
