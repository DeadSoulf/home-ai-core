package windowsclient

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func queueFixture(t *testing.T) (*Queue, string, string) {
	t.Helper()
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	for name, contents := range map[string]string{"a.txt": "first", "b.txt": "second"} {
		if err := os.WriteFile(filepath.Join(source, name), []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	filename := filepath.Join(dir, "queue.json")
	q, err := OpenQueue(filename)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = q.Close() })
	if _, err := q.Add("https://EXAMPLE.com:443/", " alice ", "folder", source, "backup", true); err != nil {
		t.Fatal(err)
	}
	return q, filename, source
}

func successfulQueueCopy(ctx context.Context, folder string, transfer Transfer, options UploadOptions) (UploadResult, error) {
	if options.Progress != nil {
		options.Progress(Progress{Path: transfer.Destination, UploadedBytes: transfer.SizeBytes, TotalBytes: transfer.SizeBytes})
	}
	return UploadResult{Path: transfer.Destination, SizeBytes: transfer.SizeBytes, SHA256: transfer.SHA256}, nil
}

func TestQueueDurableProfilePlanAndCompletedEntries(t *testing.T) {
	q, filename, _ := queueFixture(t)
	if profile := q.Profile(); profile != (QueueProfile{ServerURL: "https://example.com", Username: "alice"}) {
		t.Fatalf("profile = %+v", profile)
	}
	jobs := q.Jobs()
	jobs[0].Transfers[0].Destination = "changed-snapshot"
	if q.Jobs()[0].Transfers[0].Destination == "changed-snapshot" {
		t.Fatal("Jobs exposes queue internals")
	}
	calls := 0
	copy := func(ctx context.Context, folder string, transfer Transfer, options UploadOptions) (UploadResult, error) {
		calls++
		if !options.RestartStale {
			t.Fatal("job stale policy lost")
		}
		return successfulQueueCopy(ctx, folder, transfer, options)
	}
	if err := q.run(context.Background(), "https://example.com:443/", copy, UploadOptions{}, func(p QueueProgress) { _ = q.Jobs(); _ = q.Profile() }, "secret-token"); err != nil {
		t.Fatal(err)
	}
	if calls != len(q.Jobs()[0].Transfers) {
		t.Fatalf("copied %d entries", calls)
	}
	if err := q.Close(); err != nil {
		t.Fatal(err)
	}
	q, err := OpenQueue(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	before := calls
	if err := q.run(context.Background(), "https://example.com", copy, UploadOptions{}, nil, ""); err != nil {
		t.Fatal(err)
	}
	if calls != before {
		t.Fatal("completed entries were copied again")
	}
	if q.Jobs()[0].Status != QueueCompleted {
		t.Fatal("completion was not durable")
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"secret-token", `"password"`, `"token"`} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("queue contains secret field: %s", forbidden)
		}
	}
	if _, err := os.Stat(filename + ".lock"); err != nil {
		t.Fatal("stable lock file should remain after close:", err)
	}
}

func TestQueueStopsFailureAndRetryPreservesCompleted(t *testing.T) {
	q, filename, _ := queueFixture(t)
	calls := 0
	copy := func(ctx context.Context, folder string, transfer Transfer, options UploadOptions) (UploadResult, error) {
		calls++
		if calls == 2 {
			return UploadResult{}, errors.New("request rejected for secret-token")
		}
		return successfulQueueCopy(ctx, folder, transfer, options)
	}
	if err := q.run(context.Background(), "https://example.com", copy, UploadOptions{}, nil, "secret-token"); err == nil {
		t.Fatal("expected failure")
	}
	if calls != 2 {
		t.Fatal("continued after failure")
	}
	job := q.Jobs()[0]
	if job.Status != QueueFailed || job.Transfers[0].Status != QueueCompleted || job.Transfers[1].Status != QueueFailed || job.Transfers[2].Status != QueuePending {
		t.Fatalf("statuses = %+v", job)
	}
	if strings.Contains(job.Error, "secret-token") {
		t.Fatal("token persisted in failure")
	}
	if err := q.run(context.Background(), "https://example.com", copy, UploadOptions{}, nil, ""); err == nil {
		t.Fatal("failed entry should require retry")
	}
	if calls != 2 {
		t.Fatal("failed job was retried implicitly")
	}
	if err := q.Close(); err != nil {
		t.Fatal(err)
	}
	q, err := OpenQueue(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	if err := q.Retry(job.ID); err != nil {
		t.Fatal(err)
	}
	if err := q.run(context.Background(), "https://example.com", copy, UploadOptions{}, nil, ""); err != nil {
		t.Fatal(err)
	}
	if calls != 4 {
		t.Fatalf("retry copied %d entries, want total 4", calls)
	}
	if q.Jobs()[0].Status != QueueCompleted {
		t.Fatal("retry did not finish")
	}
	if err := q.Retry(job.ID); err == nil {
		t.Fatal("completed job should not retry")
	}
}

func TestQueueCancellationAndRunningRecovery(t *testing.T) {
	q, filename, _ := queueFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	copy := func(ctx context.Context, folder string, transfer Transfer, options UploadOptions) (UploadResult, error) {
		calls++
		if transfer.Kind == "directory" {
			return successfulQueueCopy(ctx, folder, transfer, options)
		}
		options.Progress(Progress{Path: transfer.Destination, UploadedBytes: 2, TotalBytes: transfer.SizeBytes, Resumed: true})
		cancel()
		return UploadResult{}, ctx.Err()
	}
	if err := q.run(ctx, "https://example.com", copy, UploadOptions{}, nil, ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error = %v", err)
	}
	job := q.Jobs()[0]
	if calls != 2 || job.Status != QueuePending || job.Transfers[1].Status != QueuePending || job.Transfers[1].UploadedBytes != 2 {
		t.Fatalf("cancel state = %+v", job)
	}
	// Simulate a checkpoint immediately before a process interruption.
	if err := q.setTransfer(0, 1, QueueRunning, "", 2); err != nil {
		t.Fatal(err)
	}
	if err := q.Close(); err != nil {
		t.Fatal(err)
	}
	q, err := OpenQueue(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	job = q.Jobs()[0]
	if job.Status != QueuePending || job.Transfers[1].Status != QueuePending || job.Transfers[0].Status != QueueCompleted {
		t.Fatalf("recovered state = %+v", job)
	}
	remaining := 0
	if err := q.run(context.Background(), "https://example.com", func(ctx context.Context, folder string, transfer Transfer, options UploadOptions) (UploadResult, error) {
		remaining++
		return successfulQueueCopy(ctx, folder, transfer, options)
	}, UploadOptions{}, nil, ""); err != nil {
		t.Fatal(err)
	}
	if remaining != 2 {
		t.Fatalf("copied %d after restart, want 2", remaining)
	}
}

func TestQueueProfileSecretsAndServerMismatch(t *testing.T) {
	q, _, source := queueFixture(t)
	for _, server := range []string{"https://user:password@example.com", "https://example.com?token=secret", "https://example.com#secret", "https://different.example"} {
		if _, err := q.Add(server, "alice", "folder", source, "other", false); err == nil {
			t.Fatalf("accepted server %q", server)
		}
	}
	if _, err := q.Add("https://example.com", "bob", "folder", source, "other", false); err == nil {
		t.Fatal("accepted different username")
	}
	called := false
	if err := q.run(context.Background(), "https://different.example", func(context.Context, string, Transfer, UploadOptions) (UploadResult, error) {
		called = true
		return UploadResult{}, nil
	}, UploadOptions{}, nil, ""); err == nil || called {
		t.Fatal("server mismatch must fail before copying")
	}
}

func TestQueueRejectsInvalidFileWithoutChangingIt(t *testing.T) {
	q, filename, _ := queueFixture(t)
	if err := q.Close(); err != nil {
		t.Fatal(err)
	}
	valid, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(map[string]any){
		"version":       func(state map[string]any) { state["version"] = 2 },
		"unknown field": func(state map[string]any) { state["password"] = "secret" },
		"missing jobs":  func(state map[string]any) { delete(state, "jobs") },
		"secret URL": func(state map[string]any) {
			state["profile"].(map[string]any)["server_url"] = "https://user:pass@example.com"
		},
		"job status":         func(state map[string]any) { state["jobs"].([]any)[0].(map[string]any)["status"] = "completed" },
		"transfer status":    func(state map[string]any) { queueJSONTransfer(state, 0)["status"] = "invalid" },
		"transfer unknown":   func(state map[string]any) { queueJSONTransfer(state, 0)["token"] = "secret" },
		"unsafe destination": func(state map[string]any) { queueJSONTransfer(state, 0)["destination"] = "../outside" },
		"invalid checksum":   func(state map[string]any) { queueJSONTransfer(state, 1)["sha256"] = "invalid" },
		"invalid progress":   func(state map[string]any) { queueJSONTransfer(state, 1)["uploaded_bytes"] = 1000000 },
		"duplicate ID":       func(state map[string]any) { queueJSONTransfer(state, 1)["id"] = queueJSONTransfer(state, 0)["id"] },
	}
	for name, modify := range tests {
		t.Run(name, func(t *testing.T) {
			var state map[string]any
			if err := json.Unmarshal(valid, &state); err != nil {
				t.Fatal(err)
			}
			modify(state)
			data, err := json.Marshal(state)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filename, data, 0600); err != nil {
				t.Fatal(err)
			}
			if queue, err := OpenQueue(filename); err == nil {
				queue.Close()
				t.Fatal("invalid queue accepted")
			}
			after, err := os.ReadFile(filename)
			if err != nil || string(after) != string(data) {
				t.Fatal("invalid queue was changed")
			}
		})
	}
	for _, data := range [][]byte{[]byte(`{"version":`), append(append([]byte{}, valid...), []byte(`{}`)...)} {
		if err := os.WriteFile(filename, data, 0600); err != nil {
			t.Fatal(err)
		}
		if queue, err := OpenQueue(filename); err == nil {
			queue.Close()
			t.Fatal("truncated or trailing JSON accepted")
		}
	}
}

func queueJSONTransfer(state map[string]any, index int) map[string]any {
	return state["jobs"].([]any)[0].(map[string]any)["transfers"].([]any)[index].(map[string]any)
}

func TestQueueLockConcurrentOpenAndProcessDeath(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "queue.json")
	q, err := OpenQueue(filename)
	if err != nil {
		t.Fatal(err)
	}
	if other, err := OpenQueue(filename); err == nil {
		other.Close()
		t.Fatal("concurrent open acquired queue")
	}
	if err := q.Close(); err != nil {
		t.Fatal(err)
	}
	child := exec.Command(os.Args[0], "-test.run=^TestQueueProcessLockHelper$")
	child.Env = append(os.Environ(), "HOME_AI_QUEUE_HELPER="+filename)
	stdout, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	child.Stderr = os.Stderr
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
	ready := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		if scanner.Scan() {
			ready <- scanner.Text()
		} else {
			ready <- ""
		}
	}()
	select {
	case line := <-ready:
		if line != "locked" {
			t.Fatalf("helper output = %q", line)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("helper did not acquire lock")
	}
	if other, err := OpenQueue(filename); err == nil {
		other.Close()
		t.Fatal("cross-process open acquired queue")
	}
	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = child.Wait()
	q, err = OpenQueue(filename)
	if err != nil {
		t.Fatal("lock remained after process death:", err)
	}
	defer q.Close()
}

func TestQueueProcessLockHelper(t *testing.T) {
	filename := os.Getenv("HOME_AI_QUEUE_HELPER")
	if filename == "" {
		return
	}
	q, err := OpenQueue(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	fmt.Println("locked")
	for {
		time.Sleep(time.Hour)
	}
}

func TestQueueSourceChangeIsFailedBeforeNetwork(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++; http.Error(w, "unexpected request", 500) }))
	defer server.Close()
	dir := t.TempDir()
	source := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(source, []byte("old content"), 0600); err != nil {
		t.Fatal(err)
	}
	q, err := OpenQueue(filepath.Join(dir, "queue.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	if _, err := q.Add(server.URL, "alice", "folder", source, "file.txt", false); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("new content"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(source, before.ModTime(), before.ModTime()); err != nil {
		t.Fatal(err)
	}
	client, err := New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client.Token = "fresh-token"
	if err := q.Run(context.Background(), client, UploadOptions{}, nil); err == nil {
		t.Fatal("changed source copied")
	}
	if requests != 0 {
		t.Fatalf("changed source made %d network requests", requests)
	}
	if q.Jobs()[0].Status != QueueFailed {
		t.Fatal("source failure not persisted")
	}
}

func TestQueueOversizeCheckpointPreservesPreviousFile(t *testing.T) {
	q, filename, _ := queueFixture(t)
	before, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	next := cloneQueueState(q.state)
	next.Jobs = append(next.Jobs, next.Jobs[0])
	// Exercise the exact persistence path using a small byte limit, avoiding a
	// 64 MiB fixture while proving rejection happens before any disk mutation.
	if err := q.persistWithLimit(next, len(before)); err == nil {
		t.Fatal("oversize checkpoint accepted")
	}
	after, err := os.ReadFile(filename)
	if err != nil || string(after) != string(before) {
		t.Fatal("previous checkpoint changed after size rejection")
	}
	leftovers, err := filepath.Glob(filepath.Join(filepath.Dir(filename), ".home-ai-queue-*"))
	if err != nil || len(leftovers) != 0 {
		t.Fatal("size rejection created a checkpoint")
	}
}
