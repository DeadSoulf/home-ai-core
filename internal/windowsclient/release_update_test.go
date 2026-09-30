package windowsclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsReleaseSourceFindsNewestCompleteDevRelease(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/releases" {
			http.NotFound(w, r)
			return
		}
		base := server.URL
		fmt.Fprintf(w, `[
			{"tag_name":"v0.1.80-dev","draft":false,"prerelease":true,"assets":[
				{"name":"home-ai-windows-client_0.1.80-dev_amd64.exe","browser_download_url":%q},
				{"name":"home-ai-windows-client_0.1.80-dev_amd64.exe.sha256","browser_download_url":%q}
			]},
			{"tag_name":"v0.1.82-dev","draft":false,"prerelease":true,"assets":[]},
			{"tag_name":"v0.1.81-dev","draft":false,"prerelease":true,"assets":[
				{"name":"home-ai-windows-client_0.1.81-dev_amd64.exe","browser_download_url":%q},
				{"name":"home-ai-windows-client_0.1.81-dev_amd64.exe.sha256","browser_download_url":%q}
			]},
			{"tag_name":"v1.0.0","draft":false,"prerelease":false,"assets":[]}
		]`,
			base+"/80.exe", base+"/80.sha",
			base+"/81.exe", base+"/81.sha",
		)
	}))
	defer server.Close()

	source := WindowsReleaseSource{APIURL: server.URL + "/releases", allowHTTP: true}
	update, err := source.FindUpdate(context.Background(), "0.1.79-dev")
	if err != nil {
		t.Fatal(err)
	}
	if update == nil || update.Version != "0.1.81-dev" ||
		update.ExecutableURL != server.URL+"/81.exe" ||
		update.ChecksumURL != server.URL+"/81.sha" {
		t.Fatalf("update = %#v", update)
	}
}

func TestWindowsReleaseSourceReturnsNoOlderUpdate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"tag_name":"v0.1.79-dev","draft":false,"prerelease":true,"assets":[]}]`)
	}))
	defer server.Close()
	source := WindowsReleaseSource{APIURL: server.URL, allowHTTP: true}
	update, err := source.FindUpdate(context.Background(), "0.1.79-dev")
	if err != nil {
		t.Fatal(err)
	}
	if update != nil {
		t.Fatalf("unexpected update: %#v", update)
	}
}

func TestWindowsReleaseSourceRejectsUnknownRunningVersion(t *testing.T) {
	source := WindowsReleaseSource{APIURL: "http://127.0.0.1:1", allowHTTP: true}
	if _, err := source.FindUpdate(context.Background(), "dev"); !errorsIs(err, ErrWindowsClientVersionUnknown) {
		t.Fatalf("error = %v", err)
	}
}

func TestWindowsReleaseSourceDownloadsVerifiedExecutable(t *testing.T) {
	payload := []byte("verified windows executable")
	sum := sha256.Sum256(payload)
	checksum := hex.EncodeToString(sum[:])
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/client.exe":
			_, _ = w.Write(payload)
		case "/client.exe.sha256":
			fmt.Fprintf(w, "%s  dist/home-ai-windows-client_0.1.80-dev_amd64.exe\n", checksum)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	source := WindowsReleaseSource{allowHTTP: true}
	download, err := source.Download(context.Background(), WindowsClientUpdate{
		Version:       "0.1.80-dev",
		ExecutableURL: server.URL + "/client.exe",
		ChecksumURL:   server.URL + "/client.exe.sha256",
	}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(download.Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(payload) || download.SHA256 != checksum ||
		filepath.Base(download.Path) != "home-ai-windows-client_0.1.80-dev_amd64.exe" {
		t.Fatalf("download = %#v, data = %q", download, data)
	}
}

func TestWindowsReleaseSourceRejectsChecksumMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".sha256") {
			fmt.Fprintln(w, strings.Repeat("0", 64))
			return
		}
		_, _ = w.Write([]byte("different"))
	}))
	defer server.Close()
	source := WindowsReleaseSource{allowHTTP: true}
	dir := t.TempDir()
	_, err := source.Download(context.Background(), WindowsClientUpdate{
		Version:       "0.1.80-dev",
		ExecutableURL: server.URL + "/client.exe",
		ChecksumURL:   server.URL + "/client.exe.sha256",
	}, dir)
	if err == nil {
		t.Fatal("checksum mismatch was accepted")
	}
	entries, readErr := os.ReadDir(dir)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("mismatched update left files: %#v", entries)
	}
}

func TestParseDevVersionComparison(t *testing.T) {
	a, err := parseDevVersion("v0.1.79-dev")
	if err != nil {
		t.Fatal(err)
	}
	b, err := parseDevVersion("0.1.80-dev")
	if err != nil {
		t.Fatal(err)
	}
	if compareDevVersion(a, b) >= 0 || compareDevVersion(b, a) <= 0 || compareDevVersion(a, a) != 0 {
		t.Fatal("unexpected dev version ordering")
	}
	for _, invalid := range []string{"dev", "0.1", "0.1.80", "../0.1.80-dev"} {
		if _, err := parseDevVersion(invalid); err == nil {
			t.Fatalf("accepted invalid version %q", invalid)
		}
	}
}

func errorsIs(err, target error) bool {
	for err != nil {
		if err == target {
			return true
		}
		type unwrapper interface{ Unwrap() error }
		next, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = next.Unwrap()
	}
	return false
}
