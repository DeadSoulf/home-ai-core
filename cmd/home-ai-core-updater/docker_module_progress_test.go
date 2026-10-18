package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDockerPullDetailsAreWhitelistedAndLayerScoped(t *testing.T) {
	cases := []struct{ input, want string }{
		{"aabbccddeeff: Downloading [======>] 12.3MB/45.6MB", "aabbccddeeff Downloading 12.3MB/45.6MB"},
		{"aabbccddeeff: Extracting [====>] 2kB/8kB", "aabbccddeeff Extracting 2kB/8kB"},
		{"aabbccddeeff: Pull complete", "aabbccddeeff Pull complete"},
		{"unauthorized: registry token=SECRET", ""},
		{"aabbccddeeff: Downloading https://example.test/secret", "aabbccddeeff Downloading"},
		{"aabbccddeeff: ERROR PASSWORD=secret", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := sanitizedDockerPullDetail(tc.input); got != tc.want {
			t.Errorf("sanitizedDockerPullDetail(%q) = %q; want %q", tc.input, got, tc.want)
		}
	}
}

func TestSplitDockerPullCRAndNewline(t *testing.T) {
	source := []byte("layer: Waiting\rlayer: Downloading 12MB/20MB\n")
	var parts []string
	for len(source) > 0 {
		size, content, err := splitDockerPullLines(source, false)
		if err != nil || size == 0 {
			t.Fatalf("split failed %d %v", size, err)
		}
		parts = append(parts, string(content))
		source = source[size:]
	}
	if strings.Join(parts, ",") != "layer: Waiting,layer: Downloading 12MB/20MB" {
		t.Fatal(parts)
	}
}

func TestDockerPullReportsProgressWithoutLeakingOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fake docker binary")
	}
	dir := t.TempDir()
	docker := filepath.Join(dir, "docker")
	source := "#!/bin/sh\n" +
		"if [ \"$1\" != pull ]; then exit 2; fi\n" +
		"printf 'aabbccddeeff: Downloading [======>] 12.3MB/45.6MB\\r'\n" +
		"printf 'access=PRIVATE_DO_NOT_EMIT\\n'\n" +
		"printf 'aabbccddeeff: Pull complete\\n'\n"
	if err := os.WriteFile(docker, []byte(source), 0o755); err != nil {
		t.Fatal(err)
	}
	var updates []string
	err := pullDockerImageWithProgress(context.Background(), docker, nil, "registry.test/demo@sha256:"+strings.Repeat("f", 64), func(stage, detail string) {
		updates = append(updates, stage+"|"+detail)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) == 0 || updates[len(updates)-1] != "downloaded|" {
		t.Fatalf("missing terminal image pull stage: %v", updates)
	}
	if !strings.Contains(strings.Join(updates, ","), "Downloading 12.3MB/45.6MB") {
		t.Fatalf("missing layer byte progress: %v", updates)
	}
	if strings.Contains(strings.Join(updates, ","), "PRIVATE_DO_NOT_EMIT") {
		t.Fatalf("raw CLI output leaked: %v", updates)
	}
}
