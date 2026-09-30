package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DeadSoulf/home-ai-core/internal/windowsclient"
)

func TestQueueCLIPlansTreeWithoutAuthentication(t *testing.T) {
	t.Setenv(passwordEnv, "")
	root := t.TempDir()
	source := filepath.Join(root, "documents")
	if err := os.MkdirAll(filepath.Join(source, "empty"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "notes.txt"), []byte("durable copy"), 0o600); err != nil {
		t.Fatal(err)
	}
	queuePath := filepath.Join(root, "queue.json")
	if err := run([]string{"queue", "add", "--queue", queuePath,
		"--server", "http://127.0.0.1:1", "--username", "alice", "--folder", "nsf_test",
		"--source", source, "--dest", "backup/documents"}); err != nil {
		t.Fatal(err)
	}
	queue, err := windowsclient.OpenQueue(queuePath)
	if err != nil {
		t.Fatal(err)
	}
	jobs := queue.Jobs()
	if len(jobs) != 1 || len(jobs[0].Transfers) != 3 || jobs[0].Status != "pending" {
		t.Fatalf("planned jobs = %#v", jobs)
	}
	if profile := queue.Profile(); profile.Username != "alice" || profile.ServerURL != "http://127.0.0.1:1" {
		t.Fatalf("profile = %#v", profile)
	}
	if err := queue.Close(); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"queue", "list", "--queue", queuePath}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"queue", "run", "--queue", queuePath}); err == nil || !strings.Contains(err.Error(), passwordEnv) {
		t.Fatalf("missing process password error = %v", err)
	}
	queue, err = windowsclient.OpenQueue(queuePath)
	if err != nil {
		t.Fatal(err)
	}
	defer queue.Close()
	if queue.Jobs()[0].Status != "pending" {
		t.Fatal("authentication failure changed the queued plan")
	}
}

func TestCopyCLIRejectsUnsafePlanBeforeLogin(t *testing.T) {
	t.Setenv(passwordEnv, "")
	source := filepath.Join(t.TempDir(), "source.txt")
	if err := os.WriteFile(source, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := run([]string{"copy", "--server", "http://127.0.0.1:1", "--username", "alice",
		"--folder", "nsf_test", "--source", source, "--dest", "backup/../escape.txt"})
	if err == nil || strings.Contains(err.Error(), passwordEnv) {
		t.Fatalf("unsafe destination did not fail during local planning: %v", err)
	}
}

func TestQueueCLIValidationAndEmptyRun(t *testing.T) {
	for _, args := range [][]string{
		{"queue"}, {"queue", "unknown"}, {"queue", "retry"},
		{"queue", "run", "--retries", "0"}, {"queue", "add", "--source", "file"},
		{"queue", "list", "unexpected"},
	} {
		if err := run(args); err == nil {
			t.Fatalf("run(%v) succeeded", args)
		}
	}
	t.Setenv(passwordEnv, "")
	if err := run([]string{"queue", "run", "--queue", filepath.Join(t.TempDir(), "empty.json")}); err != nil {
		t.Fatalf("empty queue requires no authentication: %v", err)
	}
}
