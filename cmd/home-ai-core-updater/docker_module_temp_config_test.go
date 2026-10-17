package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPinnedNVRBuildUsesTemporaryDockerConfigInsteadOfReadOnlyRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test needs a POSIX fake Docker CLI")
	}

	// Mimic systemd ProtectHome: the helper cannot create ~/.docker, and
	// the caller might even have a pre-existing, unusable DOCKER_CONFIG.
	protectedHome := filepath.Join(t.TempDir(), "protected-home")
	if err := os.Mkdir(protectedHome, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", protectedHome)
	t.Setenv("DOCKER_CONFIG", filepath.Join(protectedHome, ".docker"))
	t.Setenv("HOME_AI_TEST_PROTECTED_HOME", protectedHome)

	dir := t.TempDir()
	logPath := filepath.Join(dir, "docker-commands.log")
	dockerPath := filepath.Join(dir, "docker")
	script := fmt.Sprintf(`#!/bin/sh
set -eu
[ -n "${DOCKER_CONFIG:-}" ] || exit 41
[ "$HOME" != "$HOME_AI_TEST_PROTECTED_HOME" ] || exit 42
[ "$DOCKER_CONFIG" = "$HOME/.docker" ] || exit 43
[ -d "$DOCKER_CONFIG" ] || exit 44
[ "$(stat -c %%a "$HOME")" = "700" ] || exit 45
[ "$(stat -c %%a "$DOCKER_CONFIG")" = "700" ] || exit 46
printf "%%s|%%s|%%s\n" "$1" "$HOME" "$DOCKER_CONFIG" >> "$HOME_AI_TEST_DOCKER_LOG"
case "$1" in
  build)
    mkdir "$DOCKER_CONFIG/buildx"
    printf "build-state" > "$DOCKER_CONFIG/buildx/state"
    exit 0;;
  image)
    [ -f "$DOCKER_CONFIG/buildx/state" ] || exit 47
    printf "%s\n"
    exit 0;;
esac
exit 48
`, testNVRImageID)
	if err := os.WriteFile(dockerPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME_AI_TEST_DOCKER_LOG", logPath)

	imageID, err := buildPinnedNVRImage(context.Background(), dockerPath)
	if err != nil || imageID != testNVRImageID {
		t.Fatalf("NVR source build with protected HOME: image=%q error=%v", imageID, err)
	}
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected build and inspect in isolated Docker config: %s", raw)
	}
	build := strings.Split(lines[0], "|")
	inspect := strings.Split(lines[1], "|")
	if len(build) != 3 || len(inspect) != 3 || build[0] != "build" || inspect[0] != "image" {
		t.Fatalf("unexpected Docker commands: %s", raw)
	}
	if build[1] == protectedHome || build[1] != inspect[1] ||
		build[2] != filepath.Join(build[1], ".docker") || build[2] != inspect[2] {
		t.Fatalf("Docker used an unsafe or inconsistent HOME/DOCKER_CONFIG: %s", raw)
	}
	if _, err := os.Lstat(build[1]); !os.IsNotExist(err) {
		t.Fatalf("temporary Docker HOME not cleaned after build: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(protectedHome, ".docker")); !os.IsNotExist(err) {
		t.Fatalf("protected original HOME was changed: %v", err)
	}
}

func TestPinnedNVRBuildCleansTemporaryDockerConfigOnFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test needs a POSIX fake Docker CLI")
	}
	dir := t.TempDir()
	dockerPath := filepath.Join(dir, "docker")
	logPath := filepath.Join(dir, "docker-config-path")
	script := `#!/bin/sh
set -eu
[ -n "${DOCKER_CONFIG:-}" ] || exit 51
echo "$DOCKER_CONFIG" > "$HOME_AI_TEST_DOCKER_CONFIG_PATH"
echo "simulated Docker build failure" >&2
exit 52
`
	if err := os.WriteFile(dockerPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME_AI_TEST_DOCKER_CONFIG_PATH", logPath)
	_, err := buildPinnedNVRImage(context.Background(), dockerPath)
	if err == nil || !strings.Contains(err.Error(), "simulated Docker build failure") {
		t.Fatalf("source build error hidden or unexpectedly ignored: %v", err)
	}
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(strings.TrimSpace(string(raw))); !os.IsNotExist(err) {
		t.Fatalf("temporary Docker config persisted after failing build: %v", err)
	}
}
