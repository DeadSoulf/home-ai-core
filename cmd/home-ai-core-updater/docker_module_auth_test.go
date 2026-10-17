package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPullOfficialImageWithoutTokenUsesAnonymousDocker(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test requires a POSIX fake Docker command")
	}
	dir := t.TempDir()
	dockerPath := filepath.Join(dir, "docker")
	logPath := filepath.Join(dir, "commands.log")
	script := strings.Join([]string{
		"#!/bin/sh",
		"printf '%s\\n' \"$*\" >> \"$HOME_AI_TEST_DOCKER_LOG\"",
		"case \"$1\" in",
		"pull) exit 0;;",
		"login) exit 98;;",
		"*) exit 99;;",
		"esac",
	}, "\n") + "\n"
	if err := os.WriteFile(dockerPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME_AI_TEST_DOCKER_LOG", logPath)
	t.Setenv("HOME_AI_MODULE_CATALOG_TOKEN", "")
	image := "ghcr.io/deadsoulf/home-ai-core-nvr@sha256:" + strings.Repeat("a", 64)
	if err := pullDockerModuleImage(context.Background(), dockerPath, image); err != nil {
		t.Fatalf("anonymous pull must be attempted without a token: %v", err)
	}
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if want := "pull " + image + "\n"; string(raw) != want {
		t.Fatalf("unexpected Docker invocation: %q, want %q", raw, want)
	}
}

func TestPullPrivateImageWithoutTokenPreservesDockerError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test requires a POSIX fake Docker command")
	}
	dir := t.TempDir()
	dockerPath := filepath.Join(dir, "docker")
	script := strings.Join([]string{
		"#!/bin/sh",
		"if [ \"$1\" = pull ]; then",
		"  echo 'denied: access to image requires authorization' >&2",
		"  exit 17",
		"fi",
		"exit 99",
	}, "\n") + "\n"
	if err := os.WriteFile(dockerPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME_AI_MODULE_CATALOG_TOKEN", "")
	image := "ghcr.io/deadsoulf/home-ai-reference-module@sha256:" + strings.Repeat("b", 64)
	err := pullDockerModuleImage(context.Background(), dockerPath, image)
	if err == nil || !strings.Contains(err.Error(), "denied: access to image requires authorization") {
		t.Fatalf("unauthorized anonymous pull must expose Docker registry error: %v", err)
	}
	if strings.Contains(err.Error(), "not configured") {
		t.Fatalf("token absence must not preempt anonymous Docker pull: %v", err)
	}
}

func TestPullGHCRWithConfiguredTokenKeepsIsolatedAuth(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test requires a POSIX fake Docker command")
	}
	dir := t.TempDir()
	dockerPath := filepath.Join(dir, "docker")
	logPath := filepath.Join(dir, "commands.log")
	script := strings.Join([]string{
		"#!/bin/sh",
		"printf '%s\\n' \"$*\" >> \"$HOME_AI_TEST_DOCKER_LOG\"",
		"[ -n \"$DOCKER_CONFIG\" ] || exit 60",
		"case \"$1\" in",
		"  login)",
		"    case \"$*\" in *--password-stdin*) ;; *) exit 61;; esac",
		"    IFS= read -r secret",
		"    [ \"$secret\" = 'test-only-fixture-secret' ] || exit 62",
		"    exit 0;;",
		"  pull) exit 0;;",
		"  *) exit 63;;",
		"esac",
	}, "\n") + "\n"
	if err := os.WriteFile(dockerPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME_AI_TEST_DOCKER_LOG", logPath)
	t.Setenv("HOME_AI_MODULE_CATALOG_TOKEN", "test-only-fixture-secret")
	t.Setenv("HOME_AI_MODULE_REGISTRY_USER", "fixture-user")
	image := "ghcr.io/deadsoulf/home-ai-core-nvr@sha256:" + strings.Repeat("a", 64)
	if err := pullDockerModuleImage(context.Background(), dockerPath, image); err != nil {
		t.Fatalf("configured credentials should still be used: %v", err)
	}
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "login ghcr.io --username fixture-user --password-stdin\n") ||
		!strings.Contains(string(raw), "pull "+image+"\n") ||
		strings.Contains(string(raw), "test-only-fixture-secret") {
		t.Fatalf("Docker auth invocation is incorrect or leaked a credential: %q", raw)
	}
}
