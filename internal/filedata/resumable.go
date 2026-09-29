package filedata

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const resumableUploadDirName = ".home-ai-uploads"

type UploadSession struct {
	ID             string    `json:"id"`
	TargetPath     string    `json:"target_path"`
	SizeBytes      int64     `json:"size_bytes"`
	ReceivedBytes  int64     `json:"received_bytes"`
	ExpectedSHA256 string    `json:"expected_sha256,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type UploadCompleteResult struct {
	Path   string `json:"path"`
	Size   int64  `json:"size_bytes"`
	SHA256 string `json:"sha256"`
}

func StartUpload(
	root, targetPath string,
	sizeBytes int64,
	expectedSHA256 string,
	now time.Time,
) (UploadSession, error) {
	if sizeBytes < 0 {
		return UploadSession{}, errors.New("upload size cannot be negative")
	}
	expectedSHA256 = strings.ToLower(strings.TrimSpace(expectedSHA256))
	if expectedSHA256 != "" {
		if len(expectedSHA256) != sha256.Size*2 {
			return UploadSession{}, errors.New("expected SHA-256 must be 64 hexadecimal characters")
		}
		if _, err := hex.DecodeString(expectedSHA256); err != nil {
			return UploadSession{}, errors.New("expected SHA-256 must be hexadecimal")
		}
	}

	parentRelative, targetName, err := splitTarget(targetPath)
	if err != nil {
		return UploadSession{}, err
	}
	parent, err := resolveExisting(root, parentRelative)
	if err != nil {
		return UploadSession{}, err
	}
	parentInfo, err := os.Lstat(parent)
	if err != nil {
		return UploadSession{}, err
	}
	if !parentInfo.IsDir() {
		return UploadSession{}, errors.New("upload parent is not a directory")
	}
	target := filepath.Join(parent, targetName)
	if _, err := os.Lstat(target); err == nil {
		return UploadSession{}, errors.New("upload target already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return UploadSession{}, err
	}

	uploadDir, err := ensureUploadSessionDir(root)
	if err != nil {
		return UploadSession{}, err
	}
	id, err := newUploadSessionID()
	if err != nil {
		return UploadSession{}, err
	}
	partPath := uploadPartPath(uploadDir, id)
	part, err := os.OpenFile(partPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return UploadSession{}, fmt.Errorf("create resumable upload data: %w", err)
	}
	if err := part.Close(); err != nil {
		_ = os.Remove(partPath)
		return UploadSession{}, err
	}

	session := UploadSession{
		ID:             id,
		TargetPath:     filepath.ToSlash(cleanRelative(targetPath)),
		SizeBytes:      sizeBytes,
		ReceivedBytes:  0,
		ExpectedSHA256: expectedSHA256,
		CreatedAt:      now.UTC(),
		UpdatedAt:      now.UTC(),
	}
	if err := writeUploadSession(uploadMetaPath(uploadDir, id), session); err != nil {
		_ = os.Remove(partPath)
		return UploadSession{}, err
	}
	return session, nil
}

func GetUpload(root, id string) (UploadSession, error) {
	uploadDir, err := uploadSessionDir(root, false)
	if err != nil {
		return UploadSession{}, err
	}
	return readUploadSession(uploadDir, id)
}

func WriteUploadChunk(
	root, id string,
	offset int64,
	source io.Reader,
	maxChunkBytes int64,
	now time.Time,
) (UploadSession, int64, error) {
	if maxChunkBytes <= 0 {
		return UploadSession{}, 0, errors.New("invalid upload chunk limit")
	}
	uploadDir, err := uploadSessionDir(root, false)
	if err != nil {
		return UploadSession{}, 0, err
	}
	session, err := readUploadSession(uploadDir, id)
	if err != nil {
		return UploadSession{}, 0, err
	}
	if offset != session.ReceivedBytes {
		return session, 0, fmt.Errorf(
			"chunk offset %d does not match received offset %d",
			offset,
			session.ReceivedBytes,
		)
	}
	if session.ReceivedBytes >= session.SizeBytes {
		return session, 0, errors.New("upload already has all expected bytes")
	}

	partPath := uploadPartPath(uploadDir, id)
	part, err := os.OpenFile(partPath, os.O_WRONLY, 0)
	if err != nil {
		return session, 0, fmt.Errorf("open resumable upload data: %w", err)
	}
	defer part.Close()

	if err := part.Truncate(session.ReceivedBytes); err != nil {
		return session, 0, fmt.Errorf("normalize upload data length: %w", err)
	}
	if _, err := part.Seek(session.ReceivedBytes, io.SeekStart); err != nil {
		return session, 0, err
	}

	remaining := session.SizeBytes - session.ReceivedBytes
	limit := maxChunkBytes
	if remaining < limit {
		limit = remaining
	}
	reader := io.LimitReader(source, limit+1)
	written, copyErr := io.Copy(part, reader)
	if copyErr != nil {
		_ = part.Truncate(session.ReceivedBytes)
		return session, written, fmt.Errorf("write upload chunk: %w", copyErr)
	}
	if written == 0 && remaining > 0 {
		return session, 0, errors.New("upload chunk is empty")
	}
	if written > limit || written > remaining {
		_ = part.Truncate(session.ReceivedBytes)
		return session, written, errors.New("upload chunk exceeds allowed size or declared upload size")
	}
	if err := part.Sync(); err != nil {
		_ = part.Truncate(session.ReceivedBytes)
		return session, written, err
	}

	previous := session.ReceivedBytes
	session.ReceivedBytes += written
	session.UpdatedAt = now.UTC()
	if err := writeUploadSession(uploadMetaPath(uploadDir, id), session); err != nil {
		_ = part.Truncate(previous)
		return session, written, err
	}
	return session, written, nil
}

func CompleteUpload(root, id string) (UploadCompleteResult, error) {
	uploadDir, err := uploadSessionDir(root, false)
	if err != nil {
		return UploadCompleteResult{}, err
	}
	session, err := readUploadSession(uploadDir, id)
	if err != nil {
		return UploadCompleteResult{}, err
	}
	if session.ReceivedBytes != session.SizeBytes {
		return UploadCompleteResult{}, fmt.Errorf(
			"upload is incomplete: received %d of %d bytes",
			session.ReceivedBytes,
			session.SizeBytes,
		)
	}

	partPath := uploadPartPath(uploadDir, id)
	part, err := os.Open(partPath)
	if err != nil {
		return UploadCompleteResult{}, fmt.Errorf("open completed upload data: %w", err)
	}
	hash := sha256.New()
	written, hashErr := io.Copy(hash, part)
	closeErr := part.Close()
	if hashErr != nil {
		return UploadCompleteResult{}, fmt.Errorf("hash completed upload: %w", hashErr)
	}
	if closeErr != nil {
		return UploadCompleteResult{}, closeErr
	}
	if written != session.SizeBytes {
		return UploadCompleteResult{}, errors.New("upload data length does not match declared size")
	}
	checksum := hex.EncodeToString(hash.Sum(nil))
	if session.ExpectedSHA256 != "" && checksum != session.ExpectedSHA256 {
		return UploadCompleteResult{}, fmt.Errorf(
			"SHA-256 mismatch: expected %s, got %s",
			session.ExpectedSHA256,
			checksum,
		)
	}

	parentRelative, targetName, err := splitTarget(session.TargetPath)
	if err != nil {
		return UploadCompleteResult{}, err
	}
	parent, err := resolveExisting(root, parentRelative)
	if err != nil {
		return UploadCompleteResult{}, err
	}
	target := filepath.Join(parent, targetName)
	if _, err := os.Lstat(target); err == nil {
		return UploadCompleteResult{}, errors.New("upload target already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return UploadCompleteResult{}, err
	}

	if err := os.Rename(partPath, target); err != nil {
		return UploadCompleteResult{}, fmt.Errorf("commit resumable upload: %w", err)
	}
	_ = os.Remove(uploadMetaPath(uploadDir, id))
	return UploadCompleteResult{
		Path:   session.TargetPath,
		Size:   session.SizeBytes,
		SHA256: checksum,
	}, nil
}

func CancelUpload(root, id string) error {
	uploadDir, err := uploadSessionDir(root, false)
	if err != nil {
		return err
	}
	if _, err := readUploadSession(uploadDir, id); err != nil {
		return err
	}
	partPath := uploadPartPath(uploadDir, id)
	metaPath := uploadMetaPath(uploadDir, id)
	if err := os.Remove(partPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove resumable upload data: %w", err)
	}
	if err := os.Remove(metaPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove resumable upload metadata: %w", err)
	}
	return nil
}

func ensureUploadSessionDir(root string) (string, error) {
	return uploadSessionDir(root, true)
}

func uploadSessionDir(root string, create bool) (string, error) {
	root = filepath.Clean(root)
	info, err := os.Lstat(root)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", errors.New("logical folder root must be a real directory")
	}
	path := filepath.Join(root, resumableUploadDirName)
	info, err = os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) && create {
		if err := os.Mkdir(path, 0o750); err != nil {
			return "", fmt.Errorf("create resumable upload directory: %w", err)
		}
		return path, nil
	}
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", errors.New("resumable upload path must be a real directory")
	}
	return path, nil
}

func readUploadSession(uploadDir, id string) (UploadSession, error) {
	if !validUploadSessionID(id) {
		return UploadSession{}, errors.New("invalid upload session identity")
	}
	data, err := os.ReadFile(uploadMetaPath(uploadDir, id))
	if err != nil {
		return UploadSession{}, fmt.Errorf("read upload session: %w", err)
	}
	var session UploadSession
	if err := json.Unmarshal(data, &session); err != nil {
		return UploadSession{}, fmt.Errorf("decode upload session: %w", err)
	}
	if session.ID != id {
		return UploadSession{}, errors.New("upload session identity mismatch")
	}
	if session.SizeBytes < 0 || session.ReceivedBytes < 0 || session.ReceivedBytes > session.SizeBytes {
		return UploadSession{}, errors.New("upload session metadata is invalid")
	}
	return session, nil
}

func writeUploadSession(path string, session UploadSession) error {
	data, err := json.Marshal(session)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".home-ai-upload-meta-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o640); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(append(data, '\n')); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func uploadPartPath(uploadDir, id string) string {
	return filepath.Join(uploadDir, id+".part")
}

func uploadMetaPath(uploadDir, id string) string {
	return filepath.Join(uploadDir, id+".json")
}

func newUploadSessionID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate upload session identity: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

func validUploadSessionID(value string) bool {
	if len(value) != 32 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
