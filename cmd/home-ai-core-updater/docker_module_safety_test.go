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

func TestModuleInstallRejectsInvalidImageBeforeDockerSetup(t *testing.T) {
	_, _, err := performDockerModuleOperation(context.Background(), updaterhelper.Request{
		Operation: "docker.module.install", ModuleID: "demo", Image: "latest",
	}, 1000, 1000)
	if err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("invalid image error = %v", err)
	}
}

func TestModuleInstallRejectsUnsupportedOperationBeforeDockerSetup(t *testing.T) {
	_, _, err := performDockerModuleOperation(context.Background(), updaterhelper.Request{
		Operation: "docker.module.unexpected", ModuleID: "demo",
	}, 1000, 1000)
	if err == nil || !strings.Contains(err.Error(), "unsupported Docker module operation") {
		t.Fatalf("unsupported operation error = %v", err)
	}
}

func TestInstallDockerModuleDoesNotRemoveExistingContainer(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses a POSIX fake Docker CLI")
	}
	dir := t.TempDir()
	logPath := filepath.Join(dir, "commands.log")
	dockerPath := filepath.Join(dir, "docker")
	script := "#!/bin/sh\n" +
		"echo \"$*\" >> \"$HOME_AI_TEST_DOCKER_LOG\"\n" +
		"case \"$1\" in\n" +
		"  inspect) printf 'true|demo\\n'; exit 0;;\n" +
		"  pull|rm|create|start) exit 89;;\n" +
		"esac\n" +
		"exit 1\n"
	if err := os.WriteFile(dockerPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HOME_AI_TEST_DOCKER_LOG", logPath)
	_, _, err := installDockerModule(context.Background(), "demo", "example.com/demo@sha256:"+strings.Repeat("a", 64), 1000, 1000)
	if err == nil || !strings.Contains(err.Error(), "in-place upgrade is disabled") {
		t.Fatalf("existing module replacement error = %v", err)
	}
	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(log), "inspect ") || strings.Contains(string(log), "\npull ") ||
		strings.Contains(string(log), "\nrm ") || strings.Contains(string(log), "\ncreate ") {
		t.Fatalf("existing module was modified: %s", log)
	}
}

func TestModuleControlDoesNotInstallMissingDocker(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the privileged helper targets Linux")
	}
	t.Setenv("PATH", t.TempDir()) // no Docker binary available
	_, _, err := performDockerModuleOperation(context.Background(), updaterhelper.Request{
		Operation: "docker.module.stop", ModuleID: "demo",
	}, 1000, 1000)
	if err == nil || !strings.Contains(err.Error(), "Docker runtime is not ready") {
		t.Fatalf("missing Docker must cause a read-only failure, got %v", err)
	}
}

func TestInstallDockerModuleRollsBackContainerThatExitsImmediately(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses a POSIX fake Docker CLI")
	}
	dir := t.TempDir()
	logPath := filepath.Join(dir, "commands.log")
	dockerPath := filepath.Join(dir, "docker")
	script := `#!/bin/sh
echo "$*" >> "$HOME_AI_TEST_DOCKER_LOG"
case "$1" in
  inspect)
    case "$*" in
      *State.Status*) printf 'fake-id|exited\n'; exit 0;;
      *) echo 'Error: No such container' >&2; exit 1;;
    esac;;
  pull|start|rm) exit 0;;
  create) echo 'fake-id'; exit 0;;
esac
exit 1
`
	if err := os.WriteFile(dockerPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HOME_AI_TEST_DOCKER_LOG", logPath)
	_, _, err := installDockerModule(context.Background(), "demo", "example.com/demo@sha256:"+strings.Repeat("a", 64), 1000, 1000)
	if err == nil || !strings.Contains(err.Error(), "stopped during startup") {
		t.Fatalf("unexpected install result: %v", err)
	}
	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(log), "\nrm -f home-ai-module-demo") {
		t.Fatalf("failed new module was not removed: %s", log)
	}
}
