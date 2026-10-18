package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAI030DeviceSelectionUsesOnlyPairedImmutableImages(t *testing.T) {
	cases := []struct {
		current string
		count   int
		want    string
	}{
		{aiCPU030Image, 0, aiCPU030Image},
		{aiCPU030Image, 1, aiGPU030Image},
		{aiCPU030Image, 8, aiGPU030Image},
		{aiGPU030Image, 2, aiGPU030Image},
		{aiGPU030Image, 0, aiCPU030Image},
		{aiCPU031Image, 0, aiCPU031Image},
		{aiCPU031Image, 1, aiGPU031Image},
		{aiCPU031Image, 8, aiGPU031Image},
		{aiGPU031Image, 2, aiGPU031Image},
		{aiGPU031Image, 0, aiCPU031Image},
		// Older images included their own GPU runtimes and never get silently upgraded.
		{"ghcr.io/deadsoulf/home-ai-core-ai@sha256:" + strings.Repeat("a", 64), 1,
			"ghcr.io/deadsoulf/home-ai-core-ai@sha256:" + strings.Repeat("a", 64)},
	}
	for _, tc := range cases {
		actual, err := aiImageForDeviceSelection(tc.current, tc.count)
		if err != nil || actual != tc.want {
			t.Errorf("image %q / GPU count %d: %q %v", tc.current, tc.count, actual, err)
		}
	}
	for _, bad := range []string{"hello", "https://example.test", "ghcr.io/attacker:latest"} {
		if _, err := aiImageForDeviceSelection(bad, 1); err == nil {
			t.Errorf("unsafe image accepted: %s", bad)
		}
	}
	if _, err := aiImageForDeviceSelection(aiCPU030Image, 9); err == nil {
		t.Fatal("too many GPUs allowed")
	}
}

func TestAIGPUImageForInstalledVersionPair(t *testing.T) {
	for _, test := range []struct{ image, target string }{
		{aiCPU030Image, aiGPU030Image},
		{aiGPU030Image, aiGPU030Image},
		{aiCPU031Image, aiGPU031Image},
		{aiGPU031Image, aiGPU031Image},
	} {
		got, err := aiGPUImageForInstalled(test.image)
		if err != nil || got != test.target {
			t.Fatalf("GPU image target %s: %q, %v", test.image, got, err)
		}
	}
	if _, err := aiGPUImageForInstalled("ghcr.io/malicious@sha256:" + strings.Repeat("c", 64)); err == nil {
		t.Fatal("untrusted image variant accepted")
	}
}

func TestAI030PinnedImageCachedRejectsNonPinnedTargets(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX Docker stub")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "docker")
	file := "#!/bin/sh\nif [ \"$1\" = image ] && [ \"$2\" = inspect ]; then\n" +
		"  printf 'sha256:%s\\n' '" + strings.Repeat("b", 64) + "'\n" +
		"else echo 'unexpected Docker operation' >&2; exit 1; fi\n"
	if err := os.WriteFile(bin, []byte(file), 0o755); err != nil {
		t.Fatal(err)
	}
	cached, err := aiPinnedImageCached(context.Background(), bin, aiGPU030Image)
	if err != nil || !cached {
		t.Fatalf("GPU image expected: %v %v", cached, err)
	}
	if _, err := aiPinnedImageCached(context.Background(), bin, "ghcr.io/other@sha256:"+strings.Repeat("a", 64)); err == nil {
		t.Fatal("accepted arbitrary accelerator image")
	}
}
