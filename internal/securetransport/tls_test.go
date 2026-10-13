package securetransport

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestPersistentCertificateAndVerifiedHTTPS(t *testing.T) {
	dir := t.TempDir()
	cfg, fingerprint, err := Config(dir, "", "")
	if err != nil {
		t.Fatal(err)
	}
	cfg2, again, err := Config(dir, "", "")
	if err != nil || fingerprint != again || len(fingerprint) != 64 {
		t.Fatalf("certificate is not persistent: %s %s %v", fingerprint, again, err)
	}
	if cfg2.MinVersion != tls.VersionTLS12 {
		t.Fatal("weak TLS minimum")
	}
	leaf, err := x509.ParseCertificate(cfg.Certificates[0].Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil {
			t.Error("plaintext request accepted")
		}
		w.Write([]byte("secure"))
	}))
	srv.TLS = cfg
	srv.StartTLS()
	defer srv.Close()
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}}
	response, err := client.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if string(body) != "secure" {
		t.Fatal(string(body))
	}
	info, err := os.Stat(filepath.Join(dir, "tls", "server.key"))
	if err != nil || info.Mode().Perm()&0o077 != 0 {
		t.Fatal("private TLS key permissions")
	}
}

func TestIncompletePairFailsClosed(t *testing.T) {
	dir := t.TempDir()
	_, _, err := Config(dir, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "tls", "server.crt")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Config(dir, "", ""); err == nil {
		t.Fatal("incomplete certificate pair silently accepted")
	}
}
