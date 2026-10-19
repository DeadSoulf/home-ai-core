package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
)

func TestModuleUpgradeRequiresTwoImmutableDigests(t *testing.T) {
	image := "ghcr.io/deadsoulf/home-ai-core-ai@sha256:" + strings.Repeat("a", 64)
	for _, request := range []updaterhelper.Request{
		{Operation: "docker.module.upgrade", ModuleID: "ai.agent", Image: "latest", PreviousImage: image},
		{Operation: "docker.module.upgrade", ModuleID: "ai.agent", Image: image, PreviousImage: "latest"},
		{Operation: "docker.module.upgrade.rollback", ModuleID: "ai.agent", Image: image, PreviousImage: ""},
		{Operation: "docker.module.upgrade.commit", ModuleID: "ai.agent", Image: image, PreviousImage: "latest"},
	} {
		_, _, err := performDockerModuleOperation(context.Background(), request, 1000, 1000)
		if err == nil || !strings.Contains(err.Error(), "immutable") {
			t.Fatalf("unsafe upgrade request accepted: %#v, err=%v", request, err)
		}
	}
}

func TestModuleUpgradeRejectsInvalidIDBeforeDockerAccess(t *testing.T) {
	_, _, err := upgradeDockerModule(context.Background(), updaterhelper.Request{Operation: "docker.module.upgrade", ModuleID: "../other"}, 1000, 1000, firstModuleProgress(nil))
	if err == nil || !strings.Contains(err.Error(), "invalid module id") {
		t.Fatalf("invalid upgrade target accepted: %v", err)
	}
}

func TestModuleUpgradeNamesAreIsolatedPerModule(t *testing.T) {
	seen := map[string]bool{aiGPUCandidate: true, aiGPUPrevious: true}
	for _, id := range []string{"ai.agent", "nvr.module", "reference.module"} {
		candidate, previous := moduleUpgradeStageNames(id)
		for _, name := range []string{candidate, previous} {
			if seen[name] { t.Fatalf("upgrade staging name collision: %s", name) }
			seen[name] = true
		}
	}
}

func TestModuleUpgradeWillNotOverwriteExistingStagingContainer(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fake Docker CLI")
	}
	dir := t.TempDir()
	binary := filepath.Join(dir, "docker")
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = inspect ]; then\n" +
		"  printf 'old-container-id\\n'\n" +
		"  exit 0\n" +
		"fi\n" +
		"exit 91\n"
	if err := os.WriteFile(binary, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	candidate, previous := moduleUpgradeStageNames("nvr.module")
	if err := ensureModuleUpgradeNamesFree(context.Background(), binary, candidate, previous); err == nil || !strings.Contains(err.Error(), "recovery") {
		t.Fatalf("staging collision ignored: %v", err)
	}
}

func TestModuleUpgradeRejectsUnexpectedPersistentDataMount(t *testing.T) {
	if runtime.GOOS == "windows" { t.Skip("POSIX fake Docker CLI") }
	dir := t.TempDir()
	original := moduleDataRoot
	moduleDataRoot = filepath.Join(dir, "module-data")
	t.Cleanup(func() { moduleDataRoot = original })
	binary := filepath.Join(dir, "docker")
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = inspect ]; then\n" +
		"  echo '[{\"Type\":\"bind\",\"Source\":\"/tmp/wrong-path\",\"Destination\":\"/data\"}]'\n" +
		"  exit 0\n" +
		"fi\n" +
		"exit 91\n"
	if err := os.WriteFile(binary, []byte(script), 0o755); err != nil { t.Fatal(err) }
	if err := verifyModuleUpgradeDataMount(context.Background(), binary, "reference.module", "home-ai-module-reference.module"); err == nil || !strings.Contains(err.Error(), "persistent bind mount") {
		t.Fatalf("unexpected /data mount accepted: %v", err)
	}
}
