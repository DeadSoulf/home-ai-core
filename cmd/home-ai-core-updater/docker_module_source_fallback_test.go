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

const testNVRImageID = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func fakeNVRDocker(t *testing.T, script string) (string, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake Docker CLI requires a POSIX shell")
	}
	dir := t.TempDir()
	dockerPath := filepath.Join(dir, "docker")
	logPath := filepath.Join(dir, "docker.log")
	if err := os.WriteFile(dockerPath, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME_AI_TEST_DOCKER_LOG", logPath)
	t.Setenv("HOME_AI_MODULE_CATALOG_TOKEN", "")
	return dockerPath, logPath
}

func TestNVRUnauthorizedGHCRFallsBackToPinnedSource(t *testing.T) {
	docker, log := fakeNVRDocker(t, fmt.Sprintf(`
printf '%%s\n' "$*" >> "$HOME_AI_TEST_DOCKER_LOG"
case "$1" in
 pull) echo 'Error response from daemon: unauthorized' >&2; exit 1;;
 build) exit 0;;
 image)
   if [ "$2" = inspect ]; then echo '%s'; exit 0; fi;;
esac
exit 99
`, testNVRImageID))
	got, fromSource, err := resolveInstallModuleImage(context.Background(), docker, "nvr.module", nvrLegacyImage)
	if err != nil || !fromSource || got != testNVRImageID {
		t.Fatalf("fallback source image = %q, fromSource=%t, error=%v", got, fromSource, err)
	}
	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	calls := string(raw)
	for _, required := range []string{
		"pull " + nvrLegacyImage,
		"build --tag " + nvrPinnedLocalTag,
		"org.opencontainers.image.source=" + nvrPinnedRepository,
		"org.opencontainers.image.revision=" + nvrPinnedRevision,
		nvrPinnedBuildContext,
		"image inspect --format {{.Id}} " + nvrPinnedLocalTag,
	} {
		if !strings.Contains(calls, required) {
			t.Fatalf("missing %q in Docker commands: %s", required, calls)
		}
	}
	if strings.Contains(calls, "login ") || strings.Contains(calls, "create ") {
		t.Fatalf("fallback preflight must not log in or install containers: %s", calls)
	}
}

func TestNVRFallbackRestrictedToLegacyImageAndAuthorizationFailures(t *testing.T) {
	docker, log := fakeNVRDocker(t, `
printf '%s\n' "$*" >> "$HOME_AI_TEST_DOCKER_LOG"
if [ "$1" = pull ]; then
  echo "$HOME_AI_TEST_PULL_ERROR" >&2
  exit 1
fi
exit 99
`)
	cases := []struct {
		name, moduleID, image, pullError string
	}{
		{"other module", "reference.module", nvrLegacyImage, "unauthorized"},
		{"unknown digest", "nvr.module", "ghcr.io/deadsoulf/home-ai-core-nvr@sha256:" + strings.Repeat("b", 64), "unauthorized"},
		{"network outage", "nvr.module", nvrLegacyImage, "connection timed out"},
		{"TLS failure", "nvr.module", nvrLegacyImage, "x509: certificate signed by unknown authority"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME_AI_TEST_PULL_ERROR", tc.pullError)
			_, fromSource, err := resolveInstallModuleImage(context.Background(), docker, tc.moduleID, tc.image)
			if err == nil || fromSource {
				t.Fatalf("fallback permitted for %s: fromSource=%t error=%v", tc.name, fromSource, err)
			}
		})
	}
	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "build ") || strings.Contains(string(raw), "create ") {
		t.Fatalf("untrusted or unrelated image triggered Docker build: %s", raw)
	}
}

func TestNVRFallbackRejectsUntrustedLocalImageID(t *testing.T) {
	docker, _ := fakeNVRDocker(t, `
case "$1" in
 pull) echo unauthorized >&2; exit 1;;
 build) exit 0;;
 image) echo 'not-a-sha256-image-id'; exit 0;;
esac
exit 99
`)
	_, fromSource, err := resolveInstallModuleImage(context.Background(), docker, "nvr.module", nvrLegacyImage)
	if err == nil || fromSource || !strings.Contains(err.Error(), "invalid immutable SHA-256 image ID") {
		t.Fatalf("invalid local image accepted: fromSource=%t error=%v", fromSource, err)
	}
}

func TestNVRFallbackCreatesContainerFromImageIDAndPreservesSource(t *testing.T) {
	docker, log := fakeNVRDocker(t, fmt.Sprintf(`
printf '%%s\n' "$*" >> "$HOME_AI_TEST_DOCKER_LOG"
case "$1" in
 inspect)
   case "$*" in
     *State.Status*) echo 'new-id|running'; exit 0;;
     *) echo 'Error: No such container' >&2; exit 1;;
   esac;;
 pull) echo 'unauthorized' >&2; exit 1;;
 build) exit 0;;
 image) echo '%s'; exit 0;;
 create) echo 'new-id'; exit 0;;
 start) exit 0;;
esac
exit 99
`, testNVRImageID))
	t.Setenv("PATH", filepath.Dir(docker)+string(os.PathListSeparator)+os.Getenv("PATH"))
	oldDataRoot := moduleDataRoot
	moduleDataRoot = filepath.Join(t.TempDir(), "module-data")
	t.Cleanup(func() { moduleDataRoot = oldDataRoot })
	status, message, err := installDockerModule(context.Background(), "nvr.module", nvrLegacyImage, os.Getuid(), os.Getgid())
	if err != nil || status.State != "running" || status.ContainerID != "new-id" {
		t.Fatalf("built-from-source NVR install failed: %#v %q %v", status, message, err)
	}
	if !strings.Contains(message, "pinned official NVR source") {
		t.Fatalf("successful source build must be identified to the user: %q", message)
	}
	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	cmds := string(raw)
	for _, required := range []string{
		"create ",
		"--label home-ai.managed=true",
		"--label home-ai.module.requested-image=" + nvrLegacyImage,
		"--label home-ai.module.source-revision=" + nvrPinnedRevision,
		testNVRImageID,
		"start home-ai-module-nvr.module",
	} {
		if !strings.Contains(cmds, required) {
			t.Fatalf("missing Docker operation %q: %s", required, cmds)
		}
	}
	if strings.Contains(cmds, "rm -f") || strings.Contains(cmds, "login ") {
		t.Fatalf("install unexpectedly removed a container or used GHCR credentials: %s", cmds)
	}
}

func TestResumeNVRFromPinnedSourceRejectsMismatchedProvenance(t *testing.T) {
	docker, log := fakeNVRDocker(t, fmt.Sprintf(`
printf '%%s\n' "$*" >> "$HOME_AI_TEST_DOCKER_LOG"
case "$1" in
 inspect)
   case "$*" in
     *json*)
       echo '{"Id":"existing-id","Image":"%s","Config":{"Image":"%s","Labels":{"home-ai.module.requested-image":"%s","home-ai.module.source-revision":"%s"}},"State":{"Status":"running"}}'; exit 0;;
     *State.Status*) echo 'existing-id|running'; exit 0;;
     *) exit 99;;
   esac;;
esac
exit 99
`, testNVRImageID, testNVRImageID, nvrLegacyImage, nvrPinnedRevision))
	status, _, err := resumeManagedModule(context.Background(), docker, "home-ai-module-nvr.module", nvrLegacyImage)
	if err != nil || status.ContainerID != "existing-id" || status.State != "running" {
		t.Fatalf("pinned locally built container cannot be adopted: %#v %v", status, err)
	}
	for _, mismatch := range []string{
		"ghcr.io/deadsoulf/home-ai-core-nvr@sha256:" + strings.Repeat("b", 64),
		"ghcr.io/deadsoulf/other@sha256:" + strings.Repeat("b", 64),
	} {
		if _, _, err := resumeManagedModule(context.Background(), docker, "home-ai-module-nvr.module", mismatch); err == nil ||
			!strings.Contains(err.Error(), "different image") {
			t.Fatalf("unrelated manifest image adopted, got error %v", err)
		}
	}
	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "rm ") || strings.Contains(string(raw), "create ") {
		t.Fatalf("source-built container recovery modified Docker state: %s", raw)
	}
}
