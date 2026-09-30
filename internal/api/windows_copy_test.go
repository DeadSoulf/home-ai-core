package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/filedata"
	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/state"
	"github.com/DeadSoulf/home-ai-core/internal/windowsclient"
)

func copyAPIHandler(t *testing.T, writable bool) (http.Handler, string) {
	t.Helper()
	poolRoot := t.TempDir()
	folderRoot := filepath.Join(poolRoot, ".home-ai", "shared", "nsf-copy")
	if err := os.MkdirAll(folderRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	sec := defaultFakeSecurity()
	sec.actor.Permissions = []string{"security.self.read"}
	sec.actor.ResourcePermissions = []security.PermissionScope{
		{Permission: "files.read", ResourceType: "file_folder", ResourceID: "nsf-copy"},
	}
	if writable {
		sec.actor.ResourcePermissions = append(sec.actor.ResourcePermissions,
			security.PermissionScope{Permission: "files.write", ResourceType: "file_folder", ResourceID: "nsf-copy"})
	}
	handler := testHandlerWithSecurity(fakeState{nasFolders: []state.NASFolderRecord{{
		ID: "nsf-copy", PoolID: "nsp-main", PoolName: "Main", PoolRoot: poolRoot,
		Name: "Family", Kind: "shared", RelativePath: "shared/nsf-copy",
	}}}, sec)
	return handler, folderRoot
}

func TestWindowsQueueResumesTreeAgainstCoreAPI(t *testing.T) {
	handler, remoteRoot := copyAPIHandler(t, true)
	var failed atomic.Bool
	var chunkRequests atomic.Int32
	var uploadsCreated atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/chunk") {
			if chunkRequests.Add(1) == 3 && !failed.Swap(true) {
				http.Error(w, "injected interruption", http.StatusServiceUnavailable)
				return
			}
		}
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/uploads") {
			uploadsCreated.Add(1)
		}
		handler.ServeHTTP(w, r)
	}))
	defer server.Close()
	client, err := windowsclient.New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client.Token = "test"
	source := filepath.Join(t.TempDir(), "documents")
	if err := os.MkdirAll(filepath.Join(source, "empty"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(source, "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	for path, payload := range map[string]string{"a.txt": "one", "nested/b.txt": "resumable data"} {
		if err := os.WriteFile(filepath.Join(source, filepath.FromSlash(path)), []byte(payload), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	queuePath := filepath.Join(t.TempDir(), "queue.json")
	queue, err := windowsclient.OpenQueue(queuePath)
	if err != nil {
		t.Fatal(err)
	}
	job, err := queue.Add(server.URL, "alice", "nsf-copy", source, "backup/documents", false)
	if err != nil {
		t.Fatal(err)
	}
	options := windowsclient.UploadOptions{ChunkSize: 3, Retries: 1}
	if err := queue.Run(context.Background(), client, options, nil); err == nil {
		t.Fatal("injected interruption did not stop the queue")
	}
	if err := queue.Close(); err != nil {
		t.Fatal(err)
	}
	partial, err := filedata.ListUploads(remoteRoot)
	if err != nil || len(partial) != 1 || partial[0].ReceivedBytes != 3 {
		t.Fatalf("partial server upload = %#v, error %v", partial, err)
	}
	queue, err = windowsclient.OpenQueue(queuePath)
	if err != nil {
		t.Fatal(err)
	}
	defer queue.Close()
	if err := queue.Retry(job.ID); err != nil {
		t.Fatal(err)
	}
	var resumed bool
	if err := queue.Run(context.Background(), client, options, func(progress windowsclient.QueueProgress) {
		resumed = resumed || progress.Resumed
	}); err != nil {
		t.Fatal(err)
	}
	if !resumed || uploadsCreated.Load() != 2 {
		t.Fatalf("resume = %v, uploads created = %d; completed file must not be resent", resumed, uploadsCreated.Load())
	}
	for path, payload := range map[string]string{"a.txt": "one", "nested/b.txt": "resumable data"} {
		data, err := os.ReadFile(filepath.Join(remoteRoot, "backup", "documents", filepath.FromSlash(path)))
		if err != nil || string(data) != payload {
			t.Fatalf("copied %s = %q, error %v", path, data, err)
		}
	}
	if info, err := os.Stat(filepath.Join(remoteRoot, "backup", "documents", "empty")); err != nil || !info.IsDir() {
		t.Fatalf("empty directory was not copied: %v", err)
	}
	if queue.Jobs()[0].Status != "completed" {
		t.Fatalf("job status = %s", queue.Jobs()[0].Status)
	}
}

func TestWindowsCopyRecoversLostCommitResponse(t *testing.T) {
	handler, remoteRoot := copyAPIHandler(t, true)
	var lost atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/complete") && !lost.Swap(true) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, r)
			if rec.Code != http.StatusOK {
				t.Errorf("commit status = %d: %s", rec.Code, rec.Body.String())
			}
			http.Error(w, "lost successful response", http.StatusServiceUnavailable)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	defer server.Close()
	client, _ := windowsclient.New(server.URL)
	client.Token = "test"
	source := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(source, []byte("verified once"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := windowsclient.PlanCopy(source, "file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.CopyTransfer(context.Background(), "nsf-copy", plan[0], windowsclient.UploadOptions{}); err != nil {
		t.Fatal(err)
	}
	if !lost.Load() {
		t.Fatal("lost commit response was not exercised")
	}
	if data, err := os.ReadFile(filepath.Join(remoteRoot, "file.txt")); err != nil || string(data) != "verified once" {
		t.Fatalf("remote file = %q, error %v", data, err)
	}
}

func TestWindowsCopyHonorsFolderWriteScope(t *testing.T) {
	handler, remoteRoot := copyAPIHandler(t, false)
	server := httptest.NewServer(handler)
	defer server.Close()
	client, _ := windowsclient.New(server.URL)
	client.Token = "test"
	source := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(source, []byte("restricted"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := windowsclient.PlanCopy(source, "file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.CopyTransfer(context.Background(), "nsf-copy", plan[0], windowsclient.UploadOptions{}); err == nil {
		t.Fatal("copy without folder write scope succeeded")
	}
	if _, err := os.Stat(filepath.Join(remoteRoot, "file.txt")); !os.IsNotExist(err) {
		t.Fatalf("read-only scope created a remote file: %v", err)
	}
}

func TestUploadCompleteAPIKeepsConcurrentTarget(t *testing.T) {
	handler, root := copyAPIHandler(t, true)
	payload := []byte("upload data")
	sum := sha256.Sum256(payload)
	upload, err := filedata.CreateUpload(root, "contended.txt", int64(len(payload)), hex.EncodeToString(sum[:]), "test", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := filedata.AppendUploadChunk(root, upload.ID, 0, hex.EncodeToString(sum[:]), strings.NewReader(string(payload)), filedata.MaxUploadChunkBytes, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "contended.txt"), []byte("keep existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/files/folders/nsf-copy/uploads/%s/complete", upload.ID), nil)
	req.Header.Set("Authorization", "Bearer test")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusConflict || body.Error.Code != "file_upload_target_exists" {
		t.Fatalf("complete conflict status = %d, body %s", rec.Code, rec.Body.String())
	}
	if data, err := os.ReadFile(filepath.Join(root, "contended.txt")); err != nil || string(data) != "keep existing" {
		t.Fatalf("concurrent target = %q, error %v", data, err)
	}
}
