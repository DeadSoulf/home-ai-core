package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDegradedNetworkAllowsStopAndRemoveOnly(t *testing.T) {
	status := dockerRuntimeStatus{Available: true, Active: true, NetworkReady: false}
	for _, op := range []string{"docker.module.stop", "docker.module.remove"} {
		if !moduleRuntimeAllows(op, status) {
			t.Fatalf("recovery blocked for %s", op)
		}
	}
	for _, op := range []string{"docker.module.start", "docker.module.restart"} {
		if moduleRuntimeAllows(op, status) {
			t.Fatal("start on broken network")
		}
	}
	if moduleRuntimeAllows("docker.module.remove", dockerRuntimeStatus{}) {
		t.Fatal("missing daemon was accepted")
	}
}

func TestResumeSameImageNeverRemovesContainer(t *testing.T) {
	dir := t.TempDir()
	docker := filepath.Join(dir, "docker")
	log := filepath.Join(dir, "commands")
	image := "example.com/demo@sha256:" + strings.Repeat("a", 64)
	script := fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$*" >> "$HOME_AI_TEST_DOCKER_LOG"
case "$1" in
 inspect)
  case "$*" in
   *json*) echo '{"Id":"old-id","Config":{"Image":"%s"},"State":{"Status":"running"}}';;
   *State.Status*) printf 'old-id|running\n';;
   *) printf 'true|demo\n';;
  esac; exit 0;;
esac
exit 89
`, image)
	if err := os.WriteFile(docker, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME_AI_TEST_DOCKER_LOG", log)
	status, _, err := resumeManagedModule(context.Background(), docker, "home-ai-module-demo", image)
	if err != nil || status.ContainerID != "old-id" || status.State != "running" {
		t.Fatalf("adoption failed: %#v %v", status, err)
	}
	data, _ := os.ReadFile(log)
	if strings.Contains(string(data), "rm ") || strings.Contains(string(data), "create ") {
		t.Fatal("adoption destroyed the existing container")
	}
}
