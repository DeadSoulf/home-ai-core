package windowsclient

import (
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestRequireHTTPSForNonLoopbackServers(t *testing.T) {
	for _, raw := range []string{"http://192.168.1.2:82", "http://server:82", "https://user:password@server:82", "https://server:82?token=x"} {
		if _, err := New(raw); err == nil {
			t.Fatalf("unsafe URL accepted: %s", raw)
		}
	}
	for _, raw := range []string{"https://server:82", "http://127.0.0.1:18082"} {
		if _, err := New(raw); err != nil {
			t.Fatalf("valid URL rejected: %s: %v", raw, err)
		}
	}
}
func TestClientUsesExplicitCAWithoutDisablingVerification(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer srv.Close()
	cert, err := x509.ParseCertificate(srv.TLS.Certificates[0].Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME_AI_CA_FILE", path)
	client, err := New(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.HTTPClient.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if client.HTTPClient.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify {
		t.Fatal("TLS verification disabled")
	}
}
func TestClientDoesNotFollowCredentialRedirects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://example.invalid/leak", http.StatusTemporaryRedirect)
	}))
	defer srv.Close()
	client, err := New(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.HTTPClient.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Fatal("redirect followed")
	}
}
