package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/frigate"
	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
)

func frigateTestRoot(t *testing.T) string {
	t.Helper()
	old := moduleDataRoot
	moduleDataRoot = filepath.Join(t.TempDir(), "modules")
	t.Cleanup(func() { moduleDataRoot = old })
	if err := os.MkdirAll(moduleDataRoot, 0750); err != nil {
		t.Fatal(err)
	}
	return moduleDataRoot
}
func frigateTestImage(letter string) string {
	return frigate.ImageRepository + "@sha256:" + strings.Repeat(letter, 64)
}
func TestFrigateProfileIsFixedAndIsolated(t *testing.T) {
	frigateTestRoot(t)
	args := frigateCreateArgs("home-ai-module-frigate.module", frigateTestImage("a"))
	joined := strings.Join(args, " ")
	for _, bad := range []string{"--privileged", "--publish", "--device", "--gpus", "docker.sock", "--network host", "SYS_ADMIN", "NET_ADMIN", "/data"} {
		if strings.Contains(joined, bad) {
			t.Fatalf("unsafe profile: %s", bad)
		}
	}
	for _, required := range []string{"--network home-ai-frigate", "--cap-drop ALL", "--security-opt no-new-privileges", "--shm-size 256m", "dst=/config", "dst=/media/frigate", "--label home-ai.frigate.profile=1"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("missing profile constraint: %s", required)
		}
	}
	if args[len(args)-1] != frigateTestImage("a") {
		t.Fatal("image is not the final immutable argument")
	}
}
func TestFrigateStoragePreservesParentModeAndConfiguration(t *testing.T) {
	root := frigateTestRoot(t)
	if err := prepareFrigateStorage(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(root)
	if err != nil || info.Mode().Perm() != 0750 {
		t.Fatal("global module directory permissions changed", err)
	}
	file := filepath.Join(frigateRoot(), "config", "config.yml")
	original := []byte("mqtt:\n  enabled: false\ncameras: {}\n# retained\n")
	if err := os.WriteFile(file, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := prepareFrigateStorage(); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(file); err != nil || string(got) != string(original) {
		t.Fatal("existing configuration overwritten", err)
	}
}
func TestFrigateStorageRejectsSymlink(t *testing.T) {
	frigateTestRoot(t)
	if err := os.Symlink(t.TempDir(), frigateRoot()); err != nil {
		t.Fatal(err)
	}
	if err := prepareFrigateStorage(); err == nil {
		t.Fatal("followed a module directory symlink")
	}
}
func TestFrigateRequestRejectsUntrustedImages(t *testing.T) {
	frigateTestRoot(t)
	for _, image := range []string{"ghcr.io/blakeblackshear/frigate:stable", "ghcr.io/attacker/frigate@sha256:" + strings.Repeat("a", 64), frigate.ImageRepository + "@sha256:" + strings.Repeat("A", 64)} {
		if err := validateFrigateRequest(updaterhelper.Request{ModuleID: frigate.ModuleID, Operation: "docker.module.install", Image: image}); err == nil {
			t.Fatal(image)
		}
	}
	if err := validateFrigateRequest(updaterhelper.Request{ModuleID: "nvr.module", Operation: "docker.module.install", Image: "other"}); err != nil {
		t.Fatal("Frigate policy changed generic modules", err)
	}
}
func TestFrigatePendingTransactionBlocksUnsafeLifecycle(t *testing.T) {
	frigateTestRoot(t)
	if err := prepareFrigateStorage(); err != nil {
		t.Fatal(err)
	}
	txn := &frigateTransaction{OldImage: frigateTestImage("a"), NewImage: frigateTestImage("b"), Phase: "prepared"}
	if err := writeFrigateTransaction(txn); err != nil {
		t.Fatal(err)
	}
	for _, operation := range []string{"docker.module.install", "docker.module.start", "docker.module.restart", "docker.module.remove"} {
		if err := validateFrigateRequest(updaterhelper.Request{ModuleID: frigate.ModuleID, Operation: operation, Image: txn.OldImage}); err == nil {
			t.Fatal("allowed during pending migration", operation)
		}
	}
	if err := validateFrigateRequest(updaterhelper.Request{ModuleID: frigate.ModuleID, Operation: "docker.module.stop"}); err != nil {
		t.Fatal("emergency stop must remain available", err)
	}
	read, err := readFrigateTransaction()
	if err != nil || read == nil || read.OldImage != txn.OldImage {
		t.Fatalf("journal round trip failed: %v %#v", err, read)
	}
}
func TestFrigateSnapshotRollbackRestoresDatabaseAndKeepsFailedState(t *testing.T) {
	frigateTestRoot(t)
	if err := prepareFrigateStorage(); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(frigateRoot(), "config")
	db := filepath.Join(config, "frigate.db")
	for _, name := range []string{"frigate.db", "frigate.db-wal", "frigate.db-shm"} {
		if err := os.WriteFile(filepath.Join(config, name), []byte("original:"+name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	media := filepath.Join(frigateRoot(), "media", "recording.mp4")
	_ = os.WriteFile(media, []byte("recording"), 0600)
	snapshot, err := frigate.CopyConfig(config, frigateBackupPath(), frigateSnapshotLimit)
	if err != nil {
		t.Fatal(err)
	}
	txn := &frigateTransaction{OldImage: frigateTestImage("a"), NewImage: frigateTestImage("b"), Phase: "snapshotted", Snapshot: snapshot}
	if err := os.WriteFile(db, []byte("migrated"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := restoreFrigateConfig(txn); err != nil {
		t.Fatal(err)
	}
	if value, err := os.ReadFile(db); err != nil || string(value) != "original:frigate.db" {
		t.Fatal("database not restored", err)
	}
	failed, err := filepath.Glob(filepath.Join(frigateRoot(), "failed-config-*", "frigate.db"))
	if err != nil || len(failed) != 1 {
		t.Fatal("failed configuration was not preserved", err)
	}
	if value, err := os.ReadFile(failed[0]); err != nil || string(value) != "migrated" {
		t.Fatal("failed DB missing", err)
	}
	if value, err := os.ReadFile(media); err != nil || string(value) != "recording" {
		t.Fatal("media archive changed during rollback", err)
	}
	if err := os.WriteFile(filepath.Join(frigateBackupPath(), "frigate.db"), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := restoreFrigateConfig(txn); err == nil {
		t.Fatal("restored an unverified backup")
	}
}
func TestFrigateHealthRequiresRealHealthyState(t *testing.T) {
	oldWindow, oldInterval := frigateHealthWindow, frigateHealthInterval
	frigateHealthWindow = 20 * time.Millisecond
	frigateHealthInterval = time.Millisecond
	t.Cleanup(func() { frigateHealthWindow = oldWindow; frigateHealthInterval = oldInterval })
	for _, state := range []string{"running|healthy", "running|", "running|unhealthy", "exited|"} {
		binary := filepath.Join(t.TempDir(), "docker")
		if err := os.WriteFile(binary, []byte("#!/bin/sh\nprintf '%s\\n' '"+state+"'\n"), 0755); err != nil {
			t.Fatal(err)
		}
		err := waitFrigateHealthy(context.Background(), binary, "fixture")
		if (state == "running|healthy") != (err == nil) {
			t.Fatalf("state %q: %v", state, err)
		}
	}
}
func frigateInspectionFixture() map[string]any {
	return map[string]any{
		"Id": "fixture", "Config": map[string]any{"Image": frigateTestImage("a"), "Labels": map[string]string{"home-ai.managed": "true", "home-ai.module.id": frigate.ModuleID, "home-ai.frigate.profile": "1"}},
		"State":      map[string]any{"Running": true, "Status": "running"},
		"HostConfig": map[string]any{"Privileged": false, "NetworkMode": frigateNetwork},
		"Mounts":     []map[string]any{{"Type": "bind", "Source": filepath.Join(frigateRoot(), "config"), "Destination": "/config", "RW": true}, {"Type": "bind", "Source": filepath.Join(frigateRoot(), "media"), "Destination": "/media/frigate", "RW": true}},
	}
}
func TestFrigateContainerVerificationRejectsUnsafeProfile(t *testing.T) {
	frigateTestRoot(t)
	for _, kind := range []string{"valid", "privileged", "host-network", "published-port", "device", "wrong-image", "wrong-mount", "wrong-label"} {
		fixture := frigateInspectionFixture()
		host := fixture["HostConfig"].(map[string]any)
		config := fixture["Config"].(map[string]any)
		switch kind {
		case "privileged":
			host["Privileged"] = true
		case "host-network":
			host["NetworkMode"] = "host"
		case "published-port":
			host["PortBindings"] = map[string]any{"5000/tcp": []any{map[string]any{"HostPort": "5000"}}}
		case "device":
			host["Devices"] = []any{map[string]any{"PathOnHost": "/dev/dri"}}
		case "wrong-image":
			config["Image"] = frigateTestImage("b")
		case "wrong-mount":
			fixture["Mounts"].([]map[string]any)[0]["Source"] = "/etc"
		case "wrong-label":
			config["Labels"].(map[string]string)["home-ai.frigate.profile"] = "unreviewed"
		}
		raw, _ := json.Marshal(fixture)
		directory := t.TempDir()
		file := filepath.Join(directory, "inspect.json")
		_ = os.WriteFile(file, raw, 0600)
		binary := filepath.Join(directory, "docker")
		_ = os.WriteFile(binary, []byte("#!/bin/sh\ncat '"+file+"'\n"), 0755)
		_, err := verifyFrigateContainer(context.Background(), binary, "fixture", frigateTestImage("a"))
		if (kind == "valid") != (err == nil) {
			t.Fatalf("profile %s: %v", kind, err)
		}
	}
}
