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

func TestCompleteUploadPreservesLateDestinationAndSession(t *testing.T) {
	for _, kind := range []string{"file", "directory", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			payload := []byte("incoming upload")
			session := createCompletedTestUpload(t, root, "target", payload)
			target := filepath.Join(root, "target")
			createUploadTestTarget(t, kind, target)
			before, err := os.Lstat(target)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := CompleteUpload(root, session.ID); !errors.Is(err, ErrUploadTargetExists) {
				t.Fatalf("CompleteUpload() error = %v, want target exists", err)
			}
			resumed, err := GetUpload(root, session.ID)
			if err != nil || resumed.ReceivedBytes != int64(len(payload)) {
				t.Fatalf("conflicted session = %#v, error = %v", resumed, err)
			}
			data, err := os.ReadFile(uploadPartPath(root, session.ID))
			if err != nil || !bytes.Equal(data, payload) {
				t.Fatalf("upload part data = %q, error = %v", data, err)
			}
			if err := CancelUpload(root, session.ID); err != nil {
				t.Fatalf("CancelUpload() error = %v", err)
			}
			after, err := os.Lstat(target)
			if err != nil || !os.SameFile(before, after) {
				t.Fatalf("existing %s replaced or removed, error = %v", kind, err)
			}
			if kind == "file" {
				data, err := os.ReadFile(target)
				if err != nil || string(data) != "keep existing contents" {
					t.Fatalf("existing file data = %q, error = %v", data, err)
				}
			}
		})
	}
}

func TestCompleteUploadCompetingSessions(t *testing.T) {
	root := t.TempDir()
	payloads := [][]byte{[]byte("first upload"), []byte("second upload")}
	sessions := []UploadSession{
		createCompletedTestUpload(t, root, "first.bin", payloads[0]),
		createCompletedTestUpload(t, root, "second.bin", payloads[1]),
	}
	// Sessions can collide if their creation races with another caller. Point
	// two complete sessions at one destination to exercise commit arbitration.
	for i := range sessions {
		sessions[i].Path = "shared.bin"
		if err := writeUploadMetadata(root, sessions[i]); err != nil {
			t.Fatal(err)
		}
	}
	type attempt struct {
		index  int
		result UploadResult
		err    error
	}
	start := make(chan struct{})
	results := make(chan attempt, len(sessions))
	for i := range sessions {
		go func(index int) {
			<-start
			result, err := CompleteUpload(root, sessions[index].ID)
			results <- attempt{index, result, err}
		}(i)
	}
	close(start)
	winner := -1
	loser := -1
	for range sessions {
		outcome := <-results
		if outcome.err == nil {
			if winner != -1 {
				t.Fatal("both upload sessions committed")
			}
			winner = outcome.index
			hash := sha256.Sum256(payloads[winner])
			if outcome.result.Path != "shared.bin" || outcome.result.SizeBytes != int64(len(payloads[winner])) ||
				outcome.result.SHA256 != hex.EncodeToString(hash[:]) {
				t.Fatalf("committed result = %#v", outcome.result)
			}
		} else {
			if !errors.Is(outcome.err, ErrUploadTargetExists) {
				t.Fatalf("competing completion error = %v", outcome.err)
			}
			loser = outcome.index
		}
	}
	if winner == -1 || loser == -1 {
		t.Fatalf("winner = %d, loser = %d", winner, loser)
	}
	if _, err := GetUpload(root, sessions[winner].ID); !errors.Is(err, ErrUploadNotFound) {
		t.Fatalf("successful upload session retained: %v", err)
	}
	if _, err := GetUpload(root, sessions[loser].ID); err != nil {
		t.Fatalf("conflicted upload session lost: %v", err)
	}
	data, err := os.ReadFile(uploadPartPath(root, sessions[loser].ID))
	if err != nil || !bytes.Equal(data, payloads[loser]) {
		t.Fatalf("conflicted part = %q, error = %v", data, err)
	}
	if err := CancelUpload(root, sessions[loser].ID); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(filepath.Join(root, "shared.bin"))
	if err != nil || !bytes.Equal(data, payloads[winner]) {
		t.Fatalf("destination data = %q, winner = %d, error = %v", data, winner, err)
	}
}

func createCompletedTestUpload(t *testing.T, root, path string, payload []byte) UploadSession {
	t.Helper()
	session, err := CreateUpload(root, path, int64(len(payload)), "", "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	session, err = AppendUploadChunk(root, session.ID, 0, "", bytes.NewReader(payload), MaxUploadChunkBytes, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return session
}
