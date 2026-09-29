package filedata

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResumableUploadCompleteWithChecksum(t *testing.T) {
	root := filepath.Join(t.TempDir(), "folder")
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o750); err != nil {
		t.Fatal(err)
	}
	payload := []byte("hello resumable upload")
	sum := sha256.Sum256(payload)
	expected := hex.EncodeToString(sum[:])
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

	session, err := StartUpload(root, "docs/file.bin", int64(len(payload)), expected, now)
	if err != nil {
		t.Fatalf("StartUpload() error = %v", err)
	}
	if session.ReceivedBytes != 0 {
		t.Fatalf("received = %d, want 0", session.ReceivedBytes)
	}

	first := payload[:7]
	session, written, err := WriteUploadChunk(root, session.ID, 0, bytes.NewReader(first), 8, now.Add(time.Second))
	if err != nil {
		t.Fatalf("first WriteUploadChunk() error = %v", err)
	}
	if written != int64(len(first)) || session.ReceivedBytes != int64(len(first)) {
		t.Fatalf("first chunk written=%d session=%#v", written, session)
	}

	reloaded, err := GetUpload(root, session.ID)
	if err != nil {
		t.Fatalf("GetUpload() error = %v", err)
	}
	if reloaded.ReceivedBytes != int64(len(first)) {
		t.Fatalf("reloaded offset = %d", reloaded.ReceivedBytes)
	}

	rest := payload[len(first):]
	session, written, err = WriteUploadChunk(
		root,
		session.ID,
		int64(len(first)),
		bytes.NewReader(rest),
		64,
		now.Add(2*time.Second),
	)
	if err != nil {
		t.Fatalf("second WriteUploadChunk() error = %v", err)
	}
	if written != int64(len(rest)) || session.ReceivedBytes != int64(len(payload)) {
		t.Fatalf("second chunk written=%d session=%#v", written, session)
	}

	result, err := CompleteUpload(root, session.ID)
	if err != nil {
		t.Fatalf("CompleteUpload() error = %v", err)
	}
	if result.Path != "docs/file.bin" || result.Size != int64(len(payload)) || result.SHA256 != expected {
		t.Fatalf("complete result = %#v", result)
	}
	data, err := os.ReadFile(filepath.Join(root, "docs", "file.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, payload) {
		t.Fatalf("completed payload = %q", data)
	}
	if _, err := GetUpload(root, session.ID); err == nil {
		t.Fatal("completed upload session still exists")
	}
}

func TestResumableUploadRejectsWrongOffsetAndOversizedChunk(t *testing.T) {
	root := filepath.Join(t.TempDir(), "folder")
	if err := os.MkdirAll(root, 0o750); err != nil {
		t.Fatal(err)
	}
	session, err := StartUpload(root, "file.bin", 5, "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := WriteUploadChunk(root, session.ID, 1, bytes.NewReader([]byte("x")), 4, time.Now()); err == nil {
		t.Fatal("wrong offset accepted")
	}
	if _, _, err := WriteUploadChunk(root, session.ID, 0, bytes.NewReader([]byte("12345")), 4, time.Now()); err == nil {
		t.Fatal("oversized chunk accepted")
	}
	reloaded, err := GetUpload(root, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.ReceivedBytes != 0 {
		t.Fatalf("failed chunks advanced session to %d", reloaded.ReceivedBytes)
	}
}

func TestResumableUploadChecksumMismatchKeepsSession(t *testing.T) {
	root := filepath.Join(t.TempDir(), "folder")
	if err := os.MkdirAll(root, 0o750); err != nil {
		t.Fatal(err)
	}
	session, err := StartUpload(
		root,
		"file.bin",
		3,
		strings.Repeat("0", 64),
		time.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := WriteUploadChunk(root, session.ID, 0, bytes.NewReader([]byte("abc")), 8, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := CompleteUpload(root, session.ID); err == nil {
		t.Fatal("checksum mismatch accepted")
	}
	if _, err := GetUpload(root, session.ID); err != nil {
		t.Fatalf("failed completion removed resumable session: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "file.bin")); !os.IsNotExist(err) {
		t.Fatalf("target unexpectedly exists: %v", err)
	}
}

func TestCancelResumableUpload(t *testing.T) {
	root := filepath.Join(t.TempDir(), "folder")
	if err := os.MkdirAll(root, 0o750); err != nil {
		t.Fatal(err)
	}
	session, err := StartUpload(root, "file.bin", 4, "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := WriteUploadChunk(root, session.ID, 0, bytes.NewReader([]byte("ab")), 8, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := CancelUpload(root, session.ID); err != nil {
		t.Fatalf("CancelUpload() error = %v", err)
	}
	if _, err := GetUpload(root, session.ID); err == nil {
		t.Fatal("cancelled upload session still exists")
	}
}

func TestResumableUploadInternalDirectoryHidden(t *testing.T) {
	root := filepath.Join(t.TempDir(), "folder")
	if err := os.MkdirAll(root, 0o750); err != nil {
		t.Fatal(err)
	}
	if _, err := StartUpload(root, "file.bin", 0, "", time.Now()); err != nil {
		t.Fatal(err)
	}
	entries, err := List(root, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name == resumableUploadDirName {
			t.Fatalf("resumable upload directory leaked into listing: %#v", entries)
		}
	}
	if _, err := List(root, resumableUploadDirName); err == nil {
		t.Fatal("reserved resumable upload directory was browsable")
	}
}
