package filedata

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResumableUploadAcrossSessionReads(t *testing.T) {
	root := filepath.Join(t.TempDir(), "folder")
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o750); err != nil {
		t.Fatal(err)
	}
	payload := []byte("hello resumable upload")
	full := sha256.Sum256(payload)
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

	session, err := CreateUpload(
		root,
		"docs/big.bin",
		int64(len(payload)),
		hex.EncodeToString(full[:]),
		"big.bin:22:123",
		now,
	)
	if err != nil {
		t.Fatalf("CreateUpload() error = %v", err)
	}

	first := payload[:7]
	firstHash := sha256.Sum256(first)
	session, err = AppendUploadChunk(
		root,
		session.ID,
		0,
		hex.EncodeToString(firstHash[:]),
		bytes.NewReader(first),
		MaxUploadChunkBytes,
		now.Add(time.Second),
	)
	if err != nil {
		t.Fatalf("AppendUploadChunk(first) error = %v", err)
	}
	if session.ReceivedBytes != int64(len(first)) || len(session.Chunks) != 1 {
		t.Fatalf("unexpected first session state: %#v", session)
	}

	resumed, err := GetUpload(root, session.ID)
	if err != nil {
		t.Fatalf("GetUpload() error = %v", err)
	}
	if resumed.ReceivedBytes != int64(len(first)) || resumed.ClientFingerprint != "big.bin:22:123" {
		t.Fatalf("unexpected resumed state: %#v", resumed)
	}

	second := payload[len(first):]
	secondHash := sha256.Sum256(second)
	resumed, err = AppendUploadChunk(
		root,
		resumed.ID,
		resumed.ReceivedBytes,
		hex.EncodeToString(secondHash[:]),
		bytes.NewReader(second),
		MaxUploadChunkBytes,
		now.Add(2*time.Second),
	)
	if err != nil {
		t.Fatalf("AppendUploadChunk(second) error = %v", err)
	}
	if resumed.ReceivedBytes != int64(len(payload)) || len(resumed.Chunks) != 2 {
		t.Fatalf("unexpected completed session state: %#v", resumed)
	}

	result, err := CompleteUpload(root, resumed.ID)
	if err != nil {
		t.Fatalf("CompleteUpload() error = %v", err)
	}
	if result.SHA256 != hex.EncodeToString(full[:]) || result.SizeBytes != int64(len(payload)) {
		t.Fatalf("unexpected result: %#v", result)
	}
	data, err := os.ReadFile(filepath.Join(root, "docs", "big.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, payload) {
		t.Fatalf("uploaded data = %q", data)
	}
	if sessions, err := ListUploads(root); err != nil || len(sessions) != 0 {
		t.Fatalf("sessions after complete = %#v err=%v", sessions, err)
	}
}

func TestUploadChunkChecksumFailureRollsBack(t *testing.T) {
	root := t.TempDir()
	session, err := CreateUpload(root, "bad.bin", 5, "", "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	_, err = AppendUploadChunk(
		root,
		session.ID,
		0,
		strings.Repeat("0", 64),
		bytes.NewBufferString("hello"),
		MaxUploadChunkBytes,
		time.Now(),
	)
	if !errors.Is(err, ErrUploadChecksumMismatch) {
		t.Fatalf("checksum error = %v", err)
	}
	resumed, err := GetUpload(root, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.ReceivedBytes != 0 || len(resumed.Chunks) != 0 {
		t.Fatalf("failed chunk advanced session: %#v", resumed)
	}
	info, err := os.Stat(uploadPartPath(root, session.ID))
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 0 {
		t.Fatalf("part size = %d, want 0", info.Size())
	}
}

func TestUploadOffsetAndChunkLimit(t *testing.T) {
	root := t.TempDir()
	session, err := CreateUpload(root, "data.bin", 6, "", "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AppendUploadChunk(root, session.ID, 1, "", bytes.NewBufferString("a"), 4, time.Now()); !errors.Is(err, ErrUploadOffsetMismatch) {
		t.Fatalf("offset error = %v", err)
	}
	if _, err := AppendUploadChunk(root, session.ID, 0, "", bytes.NewBufferString("12345"), 4, time.Now()); !errors.Is(err, ErrUploadChunkTooLarge) {
		t.Fatalf("chunk limit error = %v", err)
	}
	resumed, err := GetUpload(root, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.ReceivedBytes != 0 {
		t.Fatalf("received bytes = %d, want 0", resumed.ReceivedBytes)
	}
}

func TestUploadTargetSafetyAndCancel(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "folder")
	if err := os.MkdirAll(root, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "exists.txt"), []byte("x"), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateUpload(root, "exists.txt", 1, "", "", time.Now()); !errors.Is(err, ErrUploadTargetExists) {
		t.Fatalf("existing target error = %v", err)
	}
	if _, err := CreateUpload(root, "../escape.bin", 1, "", "", time.Now()); err == nil {
		t.Fatal("traversal upload session succeeded")
	}

	session, err := CreateUpload(root, "new.bin", 1, "", "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CreateUpload(root, "new.bin", 1, "", "", time.Now()); !errors.Is(err, ErrUploadAlreadyActive) {
		t.Fatalf("duplicate active upload error = %v", err)
	}
	if err := CancelUpload(root, session.ID); err != nil {
		t.Fatalf("CancelUpload() error = %v", err)
	}
	if _, err := GetUpload(root, session.ID); !errors.Is(err, ErrUploadNotFound) {
		t.Fatalf("GetUpload after cancel = %v", err)
	}
}

func TestListHidesUploadArtifacts(t *testing.T) {
	root := t.TempDir()
	if _, err := CreateUpload(root, "new.bin", 1, "", "", time.Now()); err != nil {
		t.Fatal(err)
	}
	entries, err := List(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("upload artifacts leaked into listing: %#v", entries)
	}
}
