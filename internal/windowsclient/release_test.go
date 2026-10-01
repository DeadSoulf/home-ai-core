package windowsclient

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverAndDownloadLatestWindowsClientRelease(t *testing.T) {
	executable := []byte("verified-windows-client")
	sum := fmt.Sprintf("%x", sha256.Sum256(executable))
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `[
				{"tag_name":"v0.1.85-dev","draft":true,"assets":[]},
				{"tag_name":"v0.1.84-dev","draft":false,"assets":[
					{"name":"home-ai-windows-client_0.1.84-dev_amd64.exe","browser_download_url":%q,"size":%d},
					{"name":"home-ai-windows-client_0.1.84-dev_amd64.exe.sha256","browser_download_url":%q,"size":96}
				]}
			]`, server.URL+"/client.exe", len(executable), server.URL+"/client.exe.sha256")
		case "/client.exe":
			_, _ = w.Write(executable)
		case "/client.exe.sha256":
			fmt.Fprintf(w, "%s  dist/home-ai-windows-client_0.1.84-dev_amd64.exe\n", sum)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	release, err := discoverLatestWindowsClientReleaseFromURL(context.Background(), server.Client(), server.URL+"/releases")
	if err != nil {
		t.Fatal(err)
	}
	if release.Version != "0.1.84-dev" {
		t.Fatalf("version = %q", release.Version)
	}
	destination := t.TempDir()
	filename, digest, err := downloadWindowsClientReleaseToDir(context.Background(), server.Client(), release, destination)
	if err != nil {
		t.Fatal(err)
	}
	if digest != sum {
		t.Fatalf("digest = %q, want %q", digest, sum)
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(executable) {
		t.Fatalf("downloaded data = %q", data)
	}
	if filepath.Base(filename) != release.ExecutableName {
		t.Fatalf("filename = %q", filename)
	}

	cached, cachedDigest, err := downloadWindowsClientReleaseToDir(context.Background(), server.Client(), release, destination)
	if err != nil {
		t.Fatal(err)
	}
	if cached != filename || cachedDigest != digest {
		t.Fatalf("cached result = %q %q, want %q %q", cached, cachedDigest, filename, digest)
	}
}

func TestDownloadWindowsClientReleaseRejectsChecksumMismatch(t *testing.T) {
	executable := []byte("tampered")
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/client.exe":
			_, _ = w.Write(executable)
		case "/client.exe.sha256":
			fmt.Fprintln(w, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  home-ai-windows-client_0.1.84-dev_amd64.exe")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	release := WindowsClientRelease{
		Version:        "0.1.84-dev",
		TagName:        "v0.1.84-dev",
		ExecutableName: "home-ai-windows-client_0.1.84-dev_amd64.exe",
		ExecutableURL:  server.URL + "/client.exe",
		ExecutableSize: int64(len(executable)),
		ChecksumName:   "home-ai-windows-client_0.1.84-dev_amd64.exe.sha256",
		ChecksumURL:    server.URL + "/client.exe.sha256",
	}
	if _, _, err := downloadWindowsClientReleaseToDir(context.Background(), server.Client(), release, t.TempDir()); err == nil {
		t.Fatal("accepted checksum mismatch")
	}
}

func TestDiscoverLatestWindowsClientReleaseRequiresMatchingAssets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"tag_name":"v0.1.84-dev","draft":false,"assets":[]}]`)
	}))
	defer server.Close()

	_, err := discoverLatestWindowsClientReleaseFromURL(context.Background(), server.Client(), server.URL)
	if !errors.Is(err, ErrNoCompatibleWindowsClientRelease) {
		t.Fatalf("err = %v", err)
	}
}

func TestParseReleaseChecksumAcceptsSha256sumPath(t *testing.T) {
	executableName := "home-ai-windows-client_0.1.84-dev_amd64.exe"
	hash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	got, err := parseReleaseChecksum(hash+"  dist/"+executableName+"\n", executableName)
	if err != nil {
		t.Fatal(err)
	}
	if got != hash {
		t.Fatalf("hash = %q", got)
	}
}
