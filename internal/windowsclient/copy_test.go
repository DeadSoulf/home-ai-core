package windowsclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/filedata"
)

func TestPlanCopyRecursiveSnapshot(t *testing.T) {
	source := filepath.Join(t.TempDir(), "family")
	for _, relative := range []string{"a", "empty"} {
		if err := os.MkdirAll(filepath.Join(source, relative), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	writeCopyFile(t, filepath.Join(source, "a", "b.txt"), "nested bytes")
	writeCopyFile(t, filepath.Join(source, "z.txt"), "last bytes")
	plan, err := PlanCopy(source, "backup\\family//.")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, transfer := range plan {
		got = append(got, transfer.Kind+":"+transfer.Destination)
		if !filepath.IsAbs(transfer.Source) {
			t.Fatalf("source is not absolute: %#v", transfer)
		}
		if transfer.Kind == "file" {
			checksum, err := hashFile(transfer.Source)
			if err != nil || checksum != transfer.SHA256 || transfer.SizeBytes == 0 || transfer.ModTimeNS == 0 {
				t.Fatalf("invalid file snapshot: %#v, err %v", transfer, err)
			}
		}
	}
	want := []string{
		"directory:backup/family", "directory:backup/family/a", "file:backup/family/a/b.txt",
		"directory:backup/family/empty", "file:backup/family/z.txt",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("plan = %#v, want %#v", got, want)
	}
	defaultPlan, err := PlanCopy(source, "")
	if err != nil || defaultPlan[0].Destination != "family" {
		t.Fatalf("default destination: %#v, %v", defaultPlan, err)
	}
	filePlan, err := PlanCopy(filepath.Join(source, "z.txt"), "")
	if err != nil || len(filePlan) != 1 || filePlan[0].Destination != "z.txt" {
		t.Fatalf("single file plan: %#v, %v", filePlan, err)
	}
}

func TestPlanCopyRejectsUnsafeDestinations(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source.txt")
	writeCopyFile(t, source, "data")
	for _, destination := range []string{
		".", "..", "../escape", "inside/../escape", "inside/../../escape", "/absolute", "\\absolute",
		"C:\\escape", "C:relative", "\\\\server\\share", "safe/C:/escape", "bad\x00name", "bad\nname",
		".home-ai-trash/item", ".home-ai-upload-secret", "nested/.home-ai-trash/item",
		"nested/.home-ai-upload-secret", "nested\\..\\escape",
	} {
		t.Run(strconv.Quote(destination), func(t *testing.T) {
			if plan, err := PlanCopy(source, destination); err == nil || plan != nil {
				t.Fatalf("PlanCopy(%q) = %#v, %v", destination, plan, err)
			}
		})
	}
}

func TestPlanCopyRejectsNamesThatCannotBePreserved(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows normalizes trailing spaces in local file names")
	}
	for _, name := range []string{"file ", ".home-ai-upload-user", ".home-ai-trash", "back\\slash", "drive:name"} {
		t.Run(name, func(t *testing.T) {
			source := t.TempDir()
			writeCopyFile(t, filepath.Join(source, name), "data")
			if plan, err := PlanCopy(source, "tree"); err == nil || plan != nil {
				t.Fatalf("accepted unpreservable source name: %#v, %v", plan, err)
			}
		})
	}
}

func TestPlanCopyRejectsSymlinksAndSpecialFiles(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "tree")
	if err := os.Mkdir(source, 0o700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(source, "file.txt")
	writeCopyFile(t, file, "data")
	link := filepath.Join(root, "link")
	if err := os.Symlink(source, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	for _, selected := range []string{link, filepath.Join(link, "file.txt")} {
		if _, err := PlanCopy(selected, "target"); err == nil {
			t.Fatalf("accepted source symlink traversal %s", selected)
		}
	}
	if err := os.Symlink(file, filepath.Join(source, "linked.txt")); err != nil {
		t.Fatal(err)
	}
	if plan, err := PlanCopy(source, "target"); err == nil || plan != nil {
		t.Fatalf("accepted tree containing symlink: %#v, %v", plan, err)
	}
	if runtime.GOOS != "windows" {
		socket := filepath.Join(root, "socket")
		listener, err := net.Listen("unix", socket)
		if err != nil {
			t.Fatal(err)
		}
		defer listener.Close()
		if _, err := PlanCopy(socket, "socket"); err == nil {
			t.Fatal("accepted source socket")
		}
	}
}

func TestCopyTransferRecursiveStructureAndIdempotence(t *testing.T) {
	source := filepath.Join(t.TempDir(), "tree")
	if err := os.MkdirAll(filepath.Join(source, "empty"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(source, "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeCopyFile(t, filepath.Join(source, "nested", "data.bin"), "recursive bytes")
	writeCopyFile(t, filepath.Join(source, "zero.bin"), "")
	plan, err := PlanCopy(source, "backup/tree")
	if err != nil {
		t.Fatal(err)
	}
	remote := newCopyTestServer(t)
	client := remote.client(t)
	for _, transfer := range plan {
		if _, err := client.CopyTransfer(context.Background(), "test", transfer, UploadOptions{ChunkSize: 4}); err != nil {
			t.Fatalf("copy %s: %v", transfer.Destination, err)
		}
	}
	info, err := os.Stat(filepath.Join(remote.root, "backup", "tree", "empty"))
	if err != nil || !info.IsDir() {
		t.Fatalf("empty directory was not copied: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(remote.root, "backup", "tree", "nested", "data.bin"))
	if err != nil || string(data) != "recursive bytes" {
		t.Fatalf("copied bytes = %q, err %v", data, err)
	}
	before := remote.writeCount()
	for _, transfer := range plan {
		if _, err := client.CopyTransfer(context.Background(), "test", transfer, UploadOptions{}); err != nil {
			t.Fatalf("repeat copy %s: %v", transfer.Destination, err)
		}
	}
	if remote.writeCount() != before {
		t.Fatal("idempotent copy performed another remote write")
	}
}

func TestCopyTransferRejectsChangedSourceBeforeRequests(t *testing.T) {
	for _, change := range []string{"size", "mtime", "hash", "symlink", "directory-kind", "directory-symlink"} {
		t.Run(change, func(t *testing.T) {
			root := t.TempDir()
			source := filepath.Join(root, "source")
			if strings.HasPrefix(change, "directory") {
				if err := os.Mkdir(source, 0o700); err != nil {
					t.Fatal(err)
				}
			} else {
				writeCopyFile(t, source, "original")
			}
			plan, err := PlanCopy(source, "target")
			if err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(source)
			if err != nil {
				t.Fatal(err)
			}
			switch change {
			case "size":
				writeCopyFile(t, source, "changed with another size")
			case "mtime":
				if err := os.Chtimes(source, info.ModTime(), info.ModTime().Add(time.Second)); err != nil {
					t.Fatal(err)
				}
			case "hash":
				writeCopyFile(t, source, "modified")
				if err := os.Chtimes(source, info.ModTime(), info.ModTime()); err != nil {
					t.Fatal(err)
				}
			case "directory-kind":
				if err := os.Remove(source); err != nil {
					t.Fatal(err)
				}
				writeCopyFile(t, source, "file")
			case "symlink", "directory-symlink":
				other := filepath.Join(root, "other")
				if change == "directory-symlink" {
					if err := os.Mkdir(other, 0o700); err != nil {
						t.Fatal(err)
					}
				} else {
					writeCopyFile(t, other, "original")
				}
				if err := os.Remove(source); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(other, source); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			}
			var requests int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				http.Error(w, "should not be contacted", http.StatusInternalServerError)
			}))
			defer server.Close()
			client, _ := New(server.URL)
			client.Token = "token"
			if _, err := client.CopyTransfer(context.Background(), "test", plan[0], UploadOptions{}); err == nil {
				t.Fatal("accepted changed source")
			}
			if requests != 0 {
				t.Fatalf("changed source caused %d remote requests", requests)
			}
		})
	}
}

func TestCopyTransferRejectsExistingConflicts(t *testing.T) {
	for _, conflict := range []string{"different-hash", "different-size", "directory-at-file", "file-at-directory", "file-ancestor", "symlink-ancestor"} {
		t.Run(conflict, func(t *testing.T) {
			source := filepath.Join(t.TempDir(), "source.txt")
			writeCopyFile(t, source, "original")
			plan, err := PlanCopy(source, "target")
			if err != nil {
				t.Fatal(err)
			}
			remote := newCopyTestServer(t)
			switch conflict {
			case "different-hash":
				writeCopyFile(t, filepath.Join(remote.root, "target"), "modified")
			case "different-size":
				writeCopyFile(t, filepath.Join(remote.root, "target"), "longer contents")
			case "directory-at-file":
				if err := os.Mkdir(filepath.Join(remote.root, "target"), 0o700); err != nil {
					t.Fatal(err)
				}
			case "file-at-directory":
				directory := t.TempDir()
				plan, err = PlanCopy(directory, "target")
				if err != nil {
					t.Fatal(err)
				}
				writeCopyFile(t, filepath.Join(remote.root, "target"), "original")
			case "file-ancestor":
				plan[0].Destination = "target/child"
				writeCopyFile(t, filepath.Join(remote.root, "target"), "original")
			case "symlink-ancestor":
				plan[0].Destination = "target/child"
				if err := os.Symlink(t.TempDir(), filepath.Join(remote.root, "target")); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			}
			if _, err := remote.client(t).CopyTransfer(context.Background(), "test", plan[0], UploadOptions{RestartStale: true}); err == nil {
				t.Fatal("accepted conflicting destination")
			}
			if remote.writeCount() != 0 {
				t.Fatal("conflicting destination caused remote writes")
			}
		})
	}
}

func TestCopyTransferRecoversLostCompletionAndPersistedRetry(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source.txt")
	writeCopyFile(t, source, "completed bytes")
	plan, err := PlanCopy(source, "target.txt")
	if err != nil {
		t.Fatal(err)
	}
	remote := newCopyTestServer(t)
	remote.loseComplete = true
	client := remote.client(t)
	result, err := client.CopyTransfer(context.Background(), "test", plan[0], UploadOptions{ChunkSize: 3})
	if err != nil || result.SHA256 != plan[0].SHA256 || result.SizeBytes != plan[0].SizeBytes {
		t.Fatalf("lost completion recovery = %#v, %v", result, err)
	}
	before := remote.writeCount()
	if _, err := client.CopyTransfer(context.Background(), "test", plan[0], UploadOptions{}); err != nil {
		t.Fatalf("persisted retry: %v", err)
	}
	if remote.writeCount() != before {
		t.Fatal("persisted retry rewrote an already completed file")
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	if _, done, err := client.VerifyTransfer(context.Background(), "test", plan[0]); err != nil || !done {
		t.Fatalf("remote verification required missing source: done %v, err %v", done, err)
	}
}

func TestCopyTransferDoesNotCommitSourceChangedDuringUpload(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source.txt")
	writeCopyFile(t, source, "original")
	plan, err := PlanCopy(source, "target.txt")
	if err != nil {
		t.Fatal(err)
	}
	remote := newCopyTestServer(t)
	remote.afterChunk = func() { writeCopyFile(t, source, "modified") }
	if _, err := remote.client(t).CopyTransfer(context.Background(), "test", plan[0], UploadOptions{}); err == nil {
		t.Fatal("accepted source changed during upload")
	}
	if _, err := os.Stat(filepath.Join(remote.root, "target.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("changed source was committed: %v", err)
	}
}

func TestVerifyTransferRequiresDownloadedBytes(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source.txt")
	writeCopyFile(t, source, "original")
	plan, err := PlanCopy(source, "target.txt")
	if err != nil {
		t.Fatal(err)
	}
	for _, response := range []string{"unavailable", "truncated", "extra-byte"} {
		t.Run(response, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/entries") {
					writeTestJSON(w, http.StatusOK, map[string]any{"entries": []copyEntry{{
						Name: "target.txt", Path: "target.txt", Kind: "file", SizeBytes: plan[0].SizeBytes,
					}}})
					return
				}
				if response == "unavailable" {
					http.Error(w, "unavailable", http.StatusServiceUnavailable)
				} else if response == "truncated" {
					_, _ = io.WriteString(w, "orig")
				} else {
					_, _ = io.WriteString(w, "original!")
				}
			}))
			defer server.Close()
			client, _ := New(server.URL)
			client.Token = "token"
			if _, done, err := client.VerifyTransfer(context.Background(), "test", plan[0]); err == nil || done {
				t.Fatalf("accepted unproven content: done %v, err %v", done, err)
			}
		})
	}
}

func TestCopyTransferRecoversReadbackLongerThanClientTimeout(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source.txt")
	writeCopyFile(t, source, "readback")
	plan, err := PlanCopy(source, "target.txt")
	if err != nil {
		t.Fatal(err)
	}
	remote := newCopyTestServer(t)
	remote.loseComplete = true
	remote.streamContent = func(w http.ResponseWriter, r *http.Request, file *os.File, _ os.FileInfo) {
		data, readErr := io.ReadAll(file)
		if readErr != nil {
			t.Error(readErr)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		for _, b := range data {
			timer := time.NewTimer(60 * time.Millisecond)
			select {
			case <-r.Context().Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			if _, err := w.Write([]byte{b}); err != nil {
				return
			}
			w.(http.Flusher).Flush()
		}
	}
	client := remote.client(t)
	client.HTTPClient.Timeout = 200 * time.Millisecond
	var contentRequests atomic.Int32
	transport := copyRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if strings.HasSuffix(request.URL.Path, "/content") {
			contentRequests.Add(1)
		}
		return http.DefaultTransport.RoundTrip(request)
	})
	client.HTTPClient.Transport = transport
	started := time.Now()
	result, err := client.CopyTransfer(context.Background(), "test", plan[0], UploadOptions{})
	if err != nil || result.SHA256 != plan[0].SHA256 {
		t.Fatalf("long readback recovery = %#v, %v", result, err)
	}
	if elapsed := time.Since(started); elapsed < 2*client.HTTPClient.Timeout {
		t.Fatalf("readback did not exceed the shared timeout: %s", elapsed)
	}
	if contentRequests.Load() != 1 {
		t.Fatalf("custom transport saw %d content requests, want 1", contentRequests.Load())
	}
	if client.HTTPClient.Timeout != 200*time.Millisecond {
		t.Fatalf("shared client timeout changed to %s", client.HTTPClient.Timeout)
	}
}

func TestVerifyTransferBoundsInactivityAndCallerCancellation(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source.txt")
	writeCopyFile(t, source, "readback")
	plan, err := PlanCopy(source, "target.txt")
	if err != nil {
		t.Fatal(err)
	}
	for _, stall := range []string{"headers", "initial-body", "after-progress", "custom-body", "caller-cancel", "caller-deadline"} {
		t.Run(stall, func(t *testing.T) {
			contentStarted := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/entries") {
					writeTestJSON(w, http.StatusOK, map[string]any{"entries": []copyEntry{{
						Name: "target.txt", Path: "target.txt", Kind: "file", SizeBytes: plan[0].SizeBytes,
					}}})
					return
				}
				if stall != "headers" {
					w.WriteHeader(http.StatusOK)
					w.(http.Flusher).Flush()
				}
				if stall == "after-progress" {
					_, _ = io.WriteString(w, "r")
					w.(http.Flusher).Flush()
				}
				close(contentStarted)
				<-r.Context().Done()
			}))
			defer server.Close()
			client, _ := New(server.URL)
			client.Token = "token"
			client.HTTPClient.Timeout = 200 * time.Millisecond
			ctx, boundCancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer boundCancel()
			var customBody *copyCountedBody
			wantError := error(context.DeadlineExceeded)
			if stall == "caller-cancel" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				defer cancel()
				client.HTTPClient.Timeout = time.Second
				wantError = context.Canceled
				go func() {
					select {
					case <-contentStarted:
						cancel()
					case <-ctx.Done():
					}
				}()
			} else if stall == "caller-deadline" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 150*time.Millisecond)
				defer cancel()
				client.HTTPClient.Timeout = time.Second
			} else if stall == "custom-body" {
				// This reader ignores the request context; closing the response
				// body must unblock its Read when the inactivity timer expires.
				reader, writer := io.Pipe()
				defer reader.Close()
				defer writer.Close()
				watchdog := time.AfterFunc(time.Second, func() { _ = reader.CloseWithError(errors.New("test watchdog interrupted stalled reader")) })
				defer watchdog.Stop()
				customBody = &copyCountedBody{ReadCloser: reader}
				client.HTTPClient.Transport = copyRoundTripFunc(func(request *http.Request) (*http.Response, error) {
					if strings.HasSuffix(request.URL.Path, "/content") {
						return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: customBody, Request: request}, nil
					}
					return http.DefaultTransport.RoundTrip(request)
				})
			}
			started := time.Now()
			_, done, err := client.VerifyTransfer(ctx, "test", plan[0])
			if done || !errors.Is(err, wantError) {
				t.Fatalf("stalled verification: done %v, err %v, want %v", done, err, wantError)
			}
			if elapsed := time.Since(started); elapsed > 2*time.Second {
				t.Fatalf("stalled verification took %s", elapsed)
			}
			if customBody != nil && customBody.closes.Load() != 1 {
				t.Fatalf("custom response body closed %d times, want once", customBody.closes.Load())
			}
		})
	}
}

type copyRoundTripFunc func(*http.Request) (*http.Response, error)

func (transport copyRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

type copyCountedBody struct {
	io.ReadCloser
	closes atomic.Int32
}

func (body *copyCountedBody) Close() error {
	body.closes.Add(1)
	return body.ReadCloser.Close()
}

type copyTestServer struct {
	root          string
	server        *httptest.Server
	mu            sync.Mutex
	writes        int
	loseComplete  bool
	afterChunk    func()
	streamContent func(http.ResponseWriter, *http.Request, *os.File, os.FileInfo)
}

func newCopyTestServer(t *testing.T) *copyTestServer {
	t.Helper()
	remote := &copyTestServer{root: t.TempDir()}
	remote.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodGet {
			remote.mu.Lock()
			remote.writes++
			remote.mu.Unlock()
		}
		endpoint := strings.TrimPrefix(r.URL.Path, "/api/v1/files/folders/test/")
		var value any
		var err error
		switch {
		case endpoint == "entries":
			entries, listErr := filedata.List(remote.root, r.URL.Query().Get("path"))
			err, value = listErr, map[string]any{"entries": entries}
		case endpoint == "directories":
			var body struct {
				Path string `json:"path"`
			}
			err = json.NewDecoder(r.Body).Decode(&body)
			if err == nil {
				err = filedata.CreateDirectory(remote.root, body.Path)
			}
			value = map[string]any{"path": body.Path}
		case endpoint == "content":
			file, info, openErr := filedata.OpenFile(remote.root, r.URL.Query().Get("path"))
			if openErr == nil {
				defer file.Close()
				if remote.streamContent != nil {
					remote.streamContent(w, r, file, info)
					return
				}
				http.ServeContent(w, r, info.Name(), info.ModTime(), file)
				return
			}
			err = openErr
		case endpoint == "uploads" && r.Method == http.MethodGet:
			uploads, listErr := filedata.ListUploads(remote.root)
			err, value = listErr, map[string]any{"uploads": uploads}
		case endpoint == "uploads" && r.Method == http.MethodPost:
			var body struct {
				Path              string `json:"path"`
				TotalBytes        int64  `json:"total_bytes"`
				SHA256            string `json:"sha256"`
				ClientFingerprint string `json:"client_fingerprint"`
			}
			err = json.NewDecoder(r.Body).Decode(&body)
			if err == nil {
				upload, createErr := filedata.CreateUpload(remote.root, body.Path, body.TotalBytes, body.SHA256, body.ClientFingerprint, time.Now())
				err, value = createErr, map[string]any{"upload": upload}
			}
		case strings.HasPrefix(endpoint, "uploads/"):
			segments := strings.Split(endpoint, "/")
			id := segments[1]
			if len(segments) == 3 && segments[2] == "chunk" {
				offset, _ := strconv.ParseInt(r.Header.Get("Upload-Offset"), 10, 64)
				upload, chunkErr := filedata.AppendUploadChunk(remote.root, id, offset, r.Header.Get("X-Chunk-SHA256"), r.Body, filedata.MaxUploadChunkBytes, time.Now())
				err, value = chunkErr, map[string]any{"upload": upload}
				if remote.afterChunk != nil {
					remote.afterChunk()
				}
			} else if len(segments) == 3 && segments[2] == "complete" {
				result, completeErr := filedata.CompleteUpload(remote.root, id)
				err, value = completeErr, map[string]any{"file": result}
				if err == nil && remote.loseComplete {
					// Commit succeeds, then a broken response models network loss.
					_, _ = io.WriteString(w, "{broken")
					return
				}
			} else if r.Method == http.MethodDelete {
				err = filedata.CancelUpload(remote.root, id)
				value = map[string]any{}
			} else {
				upload, getErr := filedata.GetUpload(remote.root, id)
				err, value = getErr, map[string]any{"upload": upload}
			}
		default:
			http.NotFound(w, r)
			return
		}
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, filedata.ErrUploadTargetExists) {
				status = http.StatusConflict
			}
			writeTestJSON(w, status, map[string]any{"error": map[string]string{"code": "copy_test_failed", "message": err.Error()}})
			return
		}
		writeTestJSON(w, http.StatusOK, value)
	}))
	t.Cleanup(remote.server.Close)
	return remote
}

func (remote *copyTestServer) client(t *testing.T) *Client {
	t.Helper()
	client, err := New(remote.server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client.Token = "token"
	return client
}

func (remote *copyTestServer) writeCount() int {
	remote.mu.Lock()
	defer remote.mu.Unlock()
	return remote.writes
}

func writeCopyFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
