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

func TestDockerLayerAggregatesOnlyCompressedKnownBytes(t *testing.T) {
	tracker := &dockerPullTracker{}
	first := tracker.report("aabbccddeeff: Downloading [====>] 12.3MB/45.6MB")
	if !strings.Contains(first, "known:12300000/45600000") {
		t.Fatalf("first known layer bytes: %q", first)
	}
	second := tracker.report("123456abcdef: Downloading [===>] 5MB/10MB")
	if !strings.Contains(second, "known:17300000/55600000") {
		t.Fatalf("parallel layer totals: %q", second)
	}
	finished := tracker.report("aabbccddeeff: Download complete")
	if !strings.Contains(finished, "known:50600000/55600000") {
		t.Fatalf("completed layer bytes: %q", finished)
	}
	extracted := tracker.report("123456abcdef: Extracting [===>] 100MB/120MB")
	if !strings.Contains(extracted, "known:50600000/55600000") {
		t.Fatalf("extraction must not count as download: %q", extracted)
	}
	if leaked := tracker.report("Authorization: Bearer PRIVATE"); leaked != "" {
		t.Fatalf("registry secrets leaked: %q", leaked)
	}
}

func TestParseDockerSizesAreDecimalAndBounded(t *testing.T) {
	for _, tc := range []struct {
		text string
		bytes int64
		valid bool
	}{
		{"12.3MB", 12300000, true},
		{"5kB", 5000, true},
		{"2GB", 2000000000, true},
		{"100B", 100, true},
		{"3KiB", 0, false},
		{"-2GB", 0, false},
		{"NaNMB", 0, false},
		{"untrusted", 0, false},
	} {
		actual, valid := parseDockerByteSize(tc.text)
		if actual != tc.bytes || valid != tc.valid {
			t.Fatalf("parseDockerByteSize(%q)=%d,%t want %d,%t", tc.text, actual, valid, tc.bytes, tc.valid)
		}
	}
}
