package windowsclient

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCompareWindowsClientVersions(t *testing.T) {
	tests := []struct {
		left, right string
		want int
	}{
		{"0.1.184", "0.1.184-dev", 1},
		{"0.1.185-dev", "0.1.184", 1},
		{"0.1.183", "0.1.184", -1},
		{"0.1.184-dev", "0.1.184-dev", 0},
		{"0.1.184", "dev", 1},
		{"invalid", "dev", 0},
	}
	for _, item := range tests {
		if got := CompareWindowsClientVersions(item.left, item.right); got != item.want {
			t.Errorf("compare(%q,%q)=%d, want %d", item.left, item.right, got, item.want)
		}
	}
}

func TestWindowsReleaseChannel(t *testing.T) {
	for _, tt := range []struct { raw, normalized string }{
		{"", ReleaseChannelBeta},
		{" BETA ", ReleaseChannelBeta},
		{"stable", ReleaseChannelStable},
	} {
		got, err := NormalizeReleaseChannel(tt.raw)
		if err != nil || got != tt.normalized {
			t.Fatalf("normalize %q = %q, %v", tt.raw, got, err)
		}
	}
	if _, err := NormalizeReleaseChannel("nightly"); err == nil {
		t.Fatal("accepted unsupported release channel")
	}
}

func TestDiscoverWindowsClientReleaseFiltersChannelAndOrdersTags(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var rows string
		for _, version := range []string{"0.1.184-dev", "0.1.182", "0.1.185-dev", "0.1.183"} {
			prerelease := len(version) > 7 && version[len(version)-4:] == "-dev"
			name := "home-ai-windows-client_" + version + "_amd64.exe"
			rows += fmt.Sprintf(`{"tag_name":"v%s","draft":false,"prerelease":%t,"assets":[
				{"name":"%s","browser_download_url":%q,"size":64},
				{"name":"%s.sha256","browser_download_url":%q,"size":96}
			]},`, version, prerelease, name, server.URL+"/binary", name, server.URL+"/digest")
		}
		fmt.Fprint(w, "["+rows[:len(rows)-1]+"]")
	}))
	defer server.Close()
	stable, err := discoverWindowsClientReleaseFromURL(context.Background(), server.Client(), server.URL, ReleaseChannelStable)
	if err != nil || stable.Version != "0.1.183" {
		t.Fatalf("stable = %+v, %v", stable, err)
	}
	beta, err := discoverWindowsClientReleaseFromURL(context.Background(), server.Client(), server.URL, ReleaseChannelBeta)
	if err != nil || beta.Version != "0.1.185-dev" {
		t.Fatalf("beta = %+v, %v", beta, err)
	}
}
