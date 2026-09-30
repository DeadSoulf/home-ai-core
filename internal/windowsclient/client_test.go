package windowsclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLoginFoldersAndResumableUploadWithRetry(t *testing.T) {
	payload := []byte("hello resumable windows client")
	full := sha256.Sum256(payload)
	fullSHA := hex.EncodeToString(full[:])

	var mu sync.Mutex
	var received []byte
	var uploadID = "0123456789abcdef0123456789abcdef"
	var chunkAttempts int
	var createdFingerprint string
	var createdExpectedSHA string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/auth/login" && r.Method == http.MethodPost:
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode login: %v", err)
			}
			if body["username"] != "alice" || body["password"] != "secret-password" ||
				body["session_mode"] != "token" {
				t.Errorf("unexpected login body: %#v", body)
			}
			writeTestJSON(w, http.StatusOK, map[string]any{"token": "token-123"})
			return

		case r.URL.Path == "/api/v1/files/folders" && r.Method == http.MethodGet:
			requireBearer(t, r)
			writeTestJSON(w, http.StatusOK, map[string]any{
				"folders": []map[string]any{{
					"id": "nsf_test",
					"name": "Family",
					"kind": "shared",
					"pool_name": "Main",
					"can_read": true,
					"can_write": true,
				}},
			})
			return

		case r.URL.Path == "/api/v1/files/folders/nsf_test/uploads" && r.Method == http.MethodGet:
			requireBearer(t, r)
			writeTestJSON(w, http.StatusOK, map[string]any{"uploads": []any{}})
			return

		case r.URL.Path == "/api/v1/files/folders/nsf_test/uploads" && r.Method == http.MethodPost:
			requireBearer(t, r)
			var body struct {
				Path string `json:"path"`
				TotalBytes int64 `json:"total_bytes"`
				SHA256 string `json:"sha256"`
				ClientFingerprint string `json:"client_fingerprint"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode create upload: %v", err)
			}
			if body.Path != "backup/data.bin" || body.TotalBytes != int64(len(payload)) {
				t.Errorf("unexpected create upload body: %#v", body)
			}
			createdFingerprint = body.ClientFingerprint
			createdExpectedSHA = body.SHA256
			writeTestJSON(w, http.StatusCreated, map[string]any{
				"upload": map[string]any{
					"id": uploadID,
					"path": body.Path,
					"total_bytes": body.TotalBytes,
					"received_bytes": 0,
					"expected_sha256": body.SHA256,
					"client_fingerprint": body.ClientFingerprint,
					"chunks": []any{},
				},
			})
			return

		case strings.HasSuffix(r.URL.Path, "/chunk") && r.Method == http.MethodPut:
			requireBearer(t, r)
			chunkAttempts++
			if chunkAttempts == 1 {
				http.Error(w, "temporary", http.StatusServiceUnavailable)
				return
			}
			offset, err := strconv.ParseInt(r.Header.Get("Upload-Offset"), 10, 64)
			if err != nil {
				t.Errorf("bad offset: %v", err)
			}
			data, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read chunk: %v", err)
			}
			sum := sha256.Sum256(data)
			if got := r.Header.Get("X-Chunk-SHA256"); got != hex.EncodeToString(sum[:]) {
				t.Errorf("chunk hash = %q", got)
			}
			mu.Lock()
			if offset != int64(len(received)) {
				t.Errorf("offset = %d, want %d", offset, len(received))
			}
			received = append(received, data...)
			next := len(received)
			mu.Unlock()
			writeTestJSON(w, http.StatusOK, map[string]any{
				"upload": map[string]any{
					"id": uploadID,
					"path": "backup/data.bin",
					"total_bytes": len(payload),
					"received_bytes": next,
					"expected_sha256": fullSHA,
					"client_fingerprint": createdFingerprint,
				},
			})
			return

		case strings.HasSuffix(r.URL.Path, "/complete") && r.Method == http.MethodPost:
			requireBearer(t, r)
			mu.Lock()
			got := append([]byte(nil), received...)
			mu.Unlock()
			if string(got) != string(payload) {
				t.Errorf("received = %q, want %q", got, payload)
			}
			writeTestJSON(w, http.StatusOK, map[string]any{
				"file": map[string]any{
					"path": "backup/data.bin",
					"size_bytes": len(payload),
					"sha256": fullSHA,
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, err := New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client.HTTPClient.Timeout = 5 * time.Second
	ctx := context.Background()
	if err := client.Login(ctx, "alice", "secret-password"); err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	folders, err := client.Folders(ctx)
	if err != nil {
		t.Fatalf("Folders() error = %v", err)
	}
	if len(folders) != 1 || folders[0].ID != "nsf_test" || !folders[0].CanWrite {
		t.Fatalf("folders = %#v", folders)
	}

	source := filepath.Join(t.TempDir(), "data.bin")
	if err := os.WriteFile(source, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	var progress []Progress
	result, err := client.UploadFile(ctx, "nsf_test", source, UploadOptions{
		Destination: "backup/data.bin",
		ChunkSize: 5,
		Retries: 3,
		Progress: func(value Progress) {
			progress = append(progress, value)
		},
	})
	if err != nil {
		t.Fatalf("UploadFile() error = %v", err)
	}
	if result.SHA256 != fullSHA || result.SizeBytes != int64(len(payload)) {
		t.Fatalf("result = %#v", result)
	}
	if createdExpectedSHA != fullSHA {
		t.Fatalf("create expected SHA = %q, want %q", createdExpectedSHA, fullSHA)
	}
	if createdFingerprint == "" {
		t.Fatal("client fingerprint was empty")
	}
	if chunkAttempts < 2 {
		t.Fatalf("chunk attempts = %d, retry was not exercised", chunkAttempts)
	}
	if len(progress) < 2 || progress[len(progress)-1].UploadedBytes != int64(len(payload)) {
		t.Fatalf("progress = %#v", progress)
	}
}

func TestUploadResumesMatchingSession(t *testing.T) {
	payload := []byte("abcdefghij")
	first := payload[:4]
	firstSum := sha256.Sum256(first)
	fullSum := sha256.Sum256(payload)
	source := filepath.Join(t.TempDir(), "resume.bin")
	if err := os.WriteFile(source, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint := fileFingerprint(info)

	var received = append([]byte(nil), first...)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/files/folders/nsf_test/uploads" && r.Method == http.MethodGet:
			writeTestJSON(w, http.StatusOK, map[string]any{"uploads": []map[string]any{{
				"id": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				"path": "resume.bin",
				"total_bytes": len(payload),
				"received_bytes": len(first),
				"expected_sha256": hex.EncodeToString(fullSum[:]),
				"client_fingerprint": fingerprint,
				"chunks": []map[string]any{{
					"offset": 0,
					"size": len(first),
					"sha256": hex.EncodeToString(firstSum[:]),
				}},
			}}})
			return
		case strings.HasSuffix(r.URL.Path, "/chunk"):
			offset, _ := strconv.ParseInt(r.Header.Get("Upload-Offset"), 10, 64)
			if offset != int64(len(first)) {
				t.Errorf("resume offset = %d, want %d", offset, len(first))
			}
			data, _ := io.ReadAll(r.Body)
			received = append(received, data...)
			writeTestJSON(w, http.StatusOK, map[string]any{"upload": map[string]any{
				"id": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				"path": "resume.bin",
				"total_bytes": len(payload),
				"received_bytes": len(received),
			}})
			return
		case strings.HasSuffix(r.URL.Path, "/complete"):
			writeTestJSON(w, http.StatusOK, map[string]any{"file": map[string]any{
				"path": "resume.bin",
				"size_bytes": len(payload),
				"sha256": hex.EncodeToString(fullSum[:]),
			}})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, _ := New(server.URL)
	client.Token = "token"
	var sawResumed bool
	_, err = client.UploadFile(context.Background(), "nsf_test", source, UploadOptions{
		Destination: "resume.bin",
		ChunkSize: 8,
		Progress: func(value Progress) {
			sawResumed = sawResumed || value.Resumed
		},
	})
	if err != nil {
		t.Fatalf("UploadFile() resume error = %v", err)
	}
	if string(received) != string(payload) {
		t.Fatalf("received = %q, want %q", received, payload)
	}
	if !sawResumed {
		t.Fatal("resumed progress flag was not reported")
	}
}

func TestNewRejectsUnsafeServerURL(t *testing.T) {
	for _, value := range []string{"", "ftp://example.test", "example.test"} {
		if _, err := New(value); err == nil {
			t.Fatalf("New(%q) succeeded", value)
		}
	}
}

func requireBearer(t *testing.T, r *http.Request) {
	t.Helper()
	if got := r.Header.Get("Authorization"); got != "Bearer token-123" {
		t.Errorf("Authorization = %q", got)
	}
}

func writeTestJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
