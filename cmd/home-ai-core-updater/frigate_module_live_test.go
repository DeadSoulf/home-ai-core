package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/frigate"
	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
)

// This opt-in integration test requires an otherwise unused Docker daemon.
// The two immutable references identify the SAME official 0.18.2 image (index
// versus platform manifest). Fault injection explicitly mutates SQLite/config;
// it does not pretend to prove compatibility with an unreleased version.
func TestFrigateDockerLifecycle(t *testing.T) {
	if os.Getenv("FRIGATE_DOCKER_TEST") != "1" {
		t.Skip("requires an isolated Docker host and explicit opt-in")
	}
	if runtime.GOOS != "linux" {
		t.Skip("Linux Docker integration")
	}
	binary, err := exec.LookPath("docker")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	name := moduleContainerName(frigate.ModuleID)
	candidate, previous := moduleUpgradeStageNames(frigate.ModuleID)
	for _, n := range []string{name, candidate, previous} {
		exists, _, err := inspectManagedContainer(ctx, binary, n, frigate.ModuleID)
		if err != nil || exists {
			t.Fatalf("isolated test refuses existing container %s: %v", n, err)
		}
	}
	if exec.CommandContext(ctx, binary, "network", "inspect", frigateNetwork).Run() == nil {
		t.Fatal("isolated test refuses an existing Frigate network")
	}
	originalRoot := moduleDataRoot
	moduleDataRoot = filepath.Join(t.TempDir(), "modules")
	testRoot := moduleDataRoot
	t.Cleanup(func() { moduleDataRoot = originalRoot })
	if err := os.MkdirAll(moduleDataRoot, 0750); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, done := context.WithTimeout(context.Background(), 45*time.Second)
		defer done()
		for _, n := range []string{name, candidate, previous} {
			exists, managed, _ := inspectManagedContainer(cleanup, binary, n, frigate.ModuleID)
			if exists && managed {
				out, _ := exec.CommandContext(cleanup, binary, "inspect", "--format", "{{json .Mounts}}", n).Output()
				if strings.Contains(string(out), testRoot) {
					_ = exec.CommandContext(cleanup, binary, "rm", "-f", n).Run()
				}
			}
		}
		_ = exec.CommandContext(cleanup, binary, "network", "rm", frigateNetwork).Run()
	})
	verified, err := frigate.NewClient(nil).Resolve(ctx, "0.18.2", runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	oldImage := frigate.ImageRepository + "@" + verified.IndexDigest
	newImage := verified.Image
	if oldImage == newImage {
		t.Skip("upstream currently serves a single manifest; two distinct verified references are required")
	}
	t.Logf("real upstream references: old(index)=%s candidate(platform)=%s", oldImage, newImage)
	progress := func(stage, detail string) {
		if stage != "download" || detail == "" {
			t.Logf("phase %s: %s", stage, detail)
		}
	}
	status, _, err := installFrigate(ctx, oldImage, progress)
	if err != nil {
		logs, _ := exec.CommandContext(ctx, binary, "logs", "--tail", "80", name).CombinedOutput()
		t.Fatalf("initial real-container install: %v\n%s", err, logs)
	}
	if status.State != "running" {
		t.Fatalf("Frigate not running: %#v", status)
	}
	if err := verifyFrigateDatabaseLocation(ctx, binary, name); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(frigateRoot(), "config", "config.yml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		configPath = filepath.Join(frigateRoot(), "config", "config.yaml")
	}
	originalConfig, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	probe := `import sqlite3; c=sqlite3.connect('/config/frigate.db'); c.execute('CREATE TABLE IF NOT EXISTS hac_rollback_probe(value TEXT)'); c.execute('DELETE FROM hac_rollback_probe'); c.execute("INSERT INTO hac_rollback_probe VALUES ('original')"); c.commit(); c.close()`
	if out, err := exec.CommandContext(ctx, binary, "exec", name, "python3", "-c", probe).CombinedOutput(); err != nil {
		t.Fatalf("seed database probe: %v %s", err, out)
	}
	request := updaterhelper.Request{Operation: "docker.module.upgrade", ModuleID: frigate.ModuleID, Image: newImage, PreviousImage: oldImage}
	t.Run("health_failure_restores_real_database_and_config", func(t *testing.T) {
		damaged := false
		report := func(stage, detail string) {
			progress(stage, detail)
			if stage == "snapshot" {
				db := filepath.Join(frigateRoot(), "config", "frigate.db")
				code := `import sqlite3,sys; c=sqlite3.connect(sys.argv[1]); c.execute("UPDATE hac_rollback_probe SET value='migrated-failure'"); c.commit(); c.close()`
				if out, err := exec.CommandContext(ctx, "python3", "-c", code, db).CombinedOutput(); err != nil {
					t.Fatalf("inject SQLite migration failure: %v %s", err, out)
				}
				if err := os.WriteFile(configPath, []byte("mqtt: [invalid YAML\n"), 0600); err != nil {
					t.Fatal(err)
				}
				damaged = true
			}
			if stage == "verify" && damaged {
				// Deterministic failed health check: terminate only this test's candidate.
				_ = exec.CommandContext(ctx, binary, "kill", candidate).Run()
			}
		}
		_, _, err := upgradeFrigate(ctx, request, report)
		if err == nil || !strings.Contains(err.Error(), "previous image, configuration and database restored") {
			t.Fatalf("expected compensated failure: %v", err)
		}
		if !damaged {
			t.Fatal("fault injection was not reached")
		}
		if _, err := verifyFrigateContainer(ctx, binary, name, oldImage); err != nil {
			t.Fatal(err)
		}
		if err := waitFrigateHealthy(ctx, binary, name); err != nil {
			t.Fatal(err)
		}
		if got, err := os.ReadFile(configPath); err != nil || string(got) != string(originalConfig) {
			t.Fatalf("configuration not restored: %v", err)
		}
		code := `import sqlite3; c=sqlite3.connect('/config/frigate.db'); print(c.execute('SELECT value FROM hac_rollback_probe').fetchone()[0]); print(c.execute('PRAGMA integrity_check').fetchone()[0]); c.close()`
		out, err := exec.CommandContext(ctx, binary, "exec", name, "python3", "-c", code).CombinedOutput()
		if err != nil || strings.TrimSpace(string(out)) != "original\nok" {
			t.Fatalf("SQLite rollback/integrity failed: %v %s", err, out)
		}
		if txn, err := readFrigateTransaction(); err != nil || txn != nil {
			t.Fatalf("failed update left transaction: %v %#v", err, txn)
		}
	})
	if t.Failed() {
		return
	}
	t.Run("interrupted_before_registry_commit_rolls_back", func(t *testing.T) {
		if _, _, err := upgradeFrigate(ctx, request, progress); err != nil {
			t.Fatal(err)
		}
		if txn, err := readFrigateTransaction(); err != nil || txn == nil || txn.Phase != "swapped" {
			t.Fatalf("pending registry commit missing: %v %#v", err, txn)
		}
		if _, _, err := recoverFrigateRegistered(ctx, oldImage); err != nil {
			t.Fatal(err)
		}
		if _, err := verifyFrigateContainer(ctx, binary, name, oldImage); err != nil {
			t.Fatal(err)
		}
	})
	if t.Failed() {
		return
	}
	t.Run("interrupted_after_registry_commit_finalizes", func(t *testing.T) {
		if _, _, err := upgradeFrigate(ctx, request, progress); err != nil {
			t.Fatal(err)
		}
		if _, _, err := recoverFrigateRegistered(ctx, newImage); err != nil {
			t.Fatal(err)
		}
		txn, err := readFrigateTransaction()
		if err != nil || txn == nil || txn.Phase != "committed" {
			t.Fatalf("commit not durable: %v %#v", err, txn)
		}
		if err := frigate.VerifyConfig(frigateBackupPath(), txn.Snapshot, frigateSnapshotLimit); err != nil {
			t.Fatal(err)
		}
		if _, err := verifyFrigateContainer(ctx, binary, name, newImage); err != nil {
			t.Fatal(err)
		}
		if exists, _, err := inspectManagedContainer(ctx, binary, previous, frigate.ModuleID); err != nil || exists {
			t.Fatalf("previous container not cleaned: %v", err)
		}
		if _, _, err := recoverFrigateRegistered(ctx, newImage); err != nil {
			t.Fatalf("commit recovery is not idempotent: %v", err)
		}
	})
}
