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
	"sort"
	"strings"
	"time"
)

const (
	uploadPrefix         = ".home-ai-upload-"
	uploadMetadataSuffix = ".json"
	uploadPartSuffix     = ".part"
	MaxUploadChunkBytes  = int64(8 << 20)
)

var (
	ErrUploadNotFound         = errors.New("upload session not found")
	ErrUploadOffsetMismatch   = errors.New("upload offset does not match received bytes")
	ErrUploadChecksumMismatch = errors.New("upload checksum mismatch")
	ErrUploadChunkTooLarge    = errors.New("upload chunk exceeds limit")
	ErrUploadTargetExists     = errors.New("upload target already exists")
	ErrUploadAlreadyActive    = errors.New("upload already active for target path")
)

type UploadChunk struct {
	Offset int64  `json:"offset"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type UploadSession struct {
	ID                string        `json:"id"`
	Path              string        `json:"path"`
	TotalBytes        int64         `json:"total_bytes"`
	ReceivedBytes     int64         `json:"received_bytes"`
	ExpectedSHA256    string        `json:"expected_sha256,omitempty"`
	ClientFingerprint string        `json:"client_fingerprint,omitempty"`
	Chunks            []UploadChunk `json:"chunks,omitempty"`
	CreatedAt         time.Time     `json:"created_at"`
	UpdatedAt         time.Time     `json:"updated_at"`
}

type UploadResult struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
}

func CreateUpload(
	root string,
	relative string,
	totalBytes int64,
	expectedSHA256 string,
	clientFingerprint string,
	now time.Time,
) (UploadSession, error) {
	if totalBytes < 0 {
		return UploadSession{}, errors.New("upload size cannot be negative")
	}
	expectedSHA256, err := normalizeSHA256(expectedSHA256)
	if err != nil {
		return UploadSession{}, err
	}
	clientFingerprint = strings.TrimSpace(clientFingerprint)
	if len(clientFingerprint) > 256 {
		return UploadSession{}, errors.New("client fingerprint is too long")
	}
	relative, _, err = validateUploadTarget(root, relative)
	if err != nil {
		return UploadSession{}, err
	}

	sessions, err := ListUploads(root)
	if err != nil {
		return UploadSession{}, err
	}
	for _, session := range sessions {
		if session.Path == relative {
			return UploadSession{}, ErrUploadAlreadyActive
		}
	}

	id, err := newUploadID()
	if err != nil {
		return UploadSession{}, err
	}
	now = now.UTC()
	session := UploadSession{
		ID:                id,
		Path:              filepath.ToSlash(relative),
		TotalBytes:        totalBytes,
		ExpectedSHA256:    expectedSHA256,
		ClientFingerprint: clientFingerprint,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	partPath := uploadPartPath(root, id)
	part, err := os.OpenFile(partPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return UploadSession{}, fmt.Errorf("create upload part: %w", err)
	}
	if err := part.Sync(); err != nil {
		_ = part.Close()
		_ = os.Remove(partPath)
		return UploadSession{}, err
	}
	if err := part.Close(); err != nil {
		_ = os.Remove(partPath)
		return UploadSession{}, err
	}
	if err := writeUploadMetadata(root, session); err != nil {
		_ = os.Remove(partPath)
		return UploadSession{}, err
	}
	return session, nil
}

func ListUploads(root string) ([]UploadSession, error) {
	info, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, errors.New("logical folder root must be a real directory")
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read upload sessions: %w", err)
	}
	result := make([]UploadSession, 0)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() ||
			!strings.HasPrefix(name, uploadPrefix) ||
			!strings.HasSuffix(name, uploadMetadataSuffix) {
			continue
		}
		id := strings.TrimSuffix(strings.TrimPrefix(name, uploadPrefix), uploadMetadataSuffix)
		if !validUploadID(id) {
			continue
		}
		session, err := GetUpload(root, id)
		if err != nil {
			continue
		}
		result = append(result, session)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].UpdatedAt.After(result[j].UpdatedAt)
	})
	return result, nil
}

func GetUpload(root, id string) (UploadSession, error) {
	if !validUploadID(id) {
		return UploadSession{}, ErrUploadNotFound
	}
	data, err := os.ReadFile(uploadMetadataPath(root, id))
	if errors.Is(err, os.ErrNotExist) {
		return UploadSession{}, ErrUploadNotFound
	}
	if err != nil {
		return UploadSession{}, fmt.Errorf("read upload metadata: %w", err)
	}

	var session UploadSession
	if err := json.Unmarshal(data, &session); err != nil {
		return UploadSession{}, fmt.Errorf("decode upload metadata: %w", err)
	}
	if session.ID != id || session.TotalBytes < 0 || session.ReceivedBytes < 0 ||
		session.ReceivedBytes > session.TotalBytes {
		return UploadSession{}, errors.New("invalid upload metadata")
	}
	if _, _, err := validateUploadTargetPath(root, session.Path); err != nil {
		return UploadSession{}, fmt.Errorf("invalid upload target metadata: %w", err)
	}
	if session.ExpectedSHA256 != "" {
		normalized, err := normalizeSHA256(session.ExpectedSHA256)
		if err != nil || normalized != session.ExpectedSHA256 {
			return UploadSession{}, errors.New("invalid upload checksum metadata")
		}
	}
	if err := validateUploadChunks(session); err != nil {
		return UploadSession{}, err
	}

	partInfo, err := os.Lstat(uploadPartPath(root, id))
	if errors.Is(err, os.ErrNotExist) {
		return UploadSession{}, ErrUploadNotFound
	}
	if err != nil {
		return UploadSession{}, err
	}
	if partInfo.Mode()&os.ModeSymlink != 0 || !partInfo.Mode().IsRegular() {
		return UploadSession{}, errors.New("upload part is not a regular file")
	}
	if partInfo.Size() != session.ReceivedBytes {
		return UploadSession{}, errors.New("upload part size does not match metadata")
	}
	return session, nil
}

func AppendUploadChunk(
	root string,
	id string,
	offset int64,
	expectedChunkSHA256 string,
	source io.Reader,
	maxBytes int64,
	now time.Time,
) (UploadSession, error) {
	session, err := GetUpload(root, id)
	if err != nil {
		return UploadSession{}, err
	}
	if offset != session.ReceivedBytes {
		return UploadSession{}, ErrUploadOffsetMismatch
	}
	if session.ReceivedBytes >= session.TotalBytes {
		return UploadSession{}, errors.New("upload already contains all expected bytes")
	}
	if maxBytes <= 0 {
		maxBytes = MaxUploadChunkBytes
	}

	expectedChunkSHA256, err = normalizeSHA256(expectedChunkSHA256)
	if err != nil {
		return UploadSession{}, fmt.Errorf("invalid chunk checksum: %w", err)
	}

	remaining := session.TotalBytes - session.ReceivedBytes
	limit := maxBytes
	if remaining < limit {
		limit = remaining
	}

	partPath := uploadPartPath(root, id)
	part, err := os.OpenFile(partPath, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return UploadSession{}, fmt.Errorf("open upload part: %w", err)
	}

	hasher := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(part, hasher), io.LimitReader(source, limit+1))
	if copyErr != nil {
		_ = part.Truncate(offset)
		_ = part.Close()
		return UploadSession{}, fmt.Errorf("write upload chunk: %w", copyErr)
	}
	if written > limit {
		_ = part.Truncate(offset)
		_ = part.Sync()
		_ = part.Close()
		return UploadSession{}, ErrUploadChunkTooLarge
	}
	if written == 0 {
		_ = part.Close()
		return UploadSession{}, errors.New("upload chunk is empty")
	}

	chunkSHA256 := hex.EncodeToString(hasher.Sum(nil))
	if expectedChunkSHA256 != "" && chunkSHA256 != expectedChunkSHA256 {
		_ = part.Truncate(offset)
		_ = part.Sync()
		_ = part.Close()
		return UploadSession{}, ErrUploadChecksumMismatch
	}
	if err := part.Sync(); err != nil {
		_ = part.Truncate(offset)
		_ = part.Close()
		return UploadSession{}, err
	}
	if err := part.Close(); err != nil {
		_ = os.Truncate(partPath, offset)
		return UploadSession{}, err
	}

	session.Chunks = append(session.Chunks, UploadChunk{
		Offset: offset,
		Size:   written,
		SHA256: chunkSHA256,
	})
	session.ReceivedBytes += written
	session.UpdatedAt = now.UTC()
	if err := writeUploadMetadata(root, session); err != nil {
		_ = os.Truncate(partPath, offset)
		return UploadSession{}, err
	}
	return session, nil
}

func CompleteUpload(root, id string) (UploadResult, error) {
	session, err := GetUpload(root, id)
	if err != nil {
		return UploadResult{}, err
	}
	if session.ReceivedBytes != session.TotalBytes {
		return UploadResult{}, errors.New("upload is incomplete")
	}

	partPath := uploadPartPath(root, id)
	part, err := os.Open(partPath)
	if err != nil {
		return UploadResult{}, err
	}
	hasher := sha256.New()
	if _, err := io.Copy(hasher, part); err != nil {
		_ = part.Close()
		return UploadResult{}, fmt.Errorf("hash upload: %w", err)
	}
	if err := part.Close(); err != nil {
		return UploadResult{}, err
	}
	fullSHA256 := hex.EncodeToString(hasher.Sum(nil))
	if session.ExpectedSHA256 != "" && fullSHA256 != session.ExpectedSHA256 {
		return UploadResult{}, ErrUploadChecksumMismatch
	}

	relative, target, err := validateUploadTarget(root, session.Path)
	if err != nil {
		return UploadResult{}, err
	}
	source, err := os.OpenFile(partPath, os.O_RDWR, 0)
	if err != nil {
		return UploadResult{}, err
	}
	if err := source.Sync(); err != nil {
		_ = source.Close()
		return UploadResult{}, err
	}
	if err := source.Close(); err != nil {
		return UploadResult{}, err
	}
	if err := os.Rename(partPath, target); err != nil {
		return UploadResult{}, fmt.Errorf("commit upload: %w", err)
	}
	if parent, err := os.Open(filepath.Dir(target)); err == nil {
		_ = parent.Sync()
		_ = parent.Close()
	}
	_ = os.Remove(uploadMetadataPath(root, id))
	return UploadResult{
		Path:      filepath.ToSlash(relative),
		SizeBytes: session.TotalBytes,
		SHA256:    fullSHA256,
	}, nil
}

func CancelUpload(root, id string) error {
	if !validUploadID(id) {
		return ErrUploadNotFound
	}
	if _, err := GetUpload(root, id); err != nil {
		return err
	}
	var firstErr error
	if err := os.Remove(uploadPartPath(root, id)); err != nil && !errors.Is(err, os.ErrNotExist) {
		firstErr = err
	}
	if err := os.Remove(uploadMetadataPath(root, id)); err != nil && !errors.Is(err, os.ErrNotExist) && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

func validateUploadChunks(session UploadSession) error {
	var received int64
	for _, chunk := range session.Chunks {
		if chunk.Offset != received || chunk.Size <= 0 {
			return errors.New("invalid upload chunk metadata")
		}
		if normalized, err := normalizeSHA256(chunk.SHA256); err != nil || normalized != chunk.SHA256 {
			return errors.New("invalid upload chunk checksum metadata")
		}
		received += chunk.Size
	}
	if received != session.ReceivedBytes {
		return errors.New("upload chunk metadata does not match received bytes")
	}
	return nil
}

func validateUploadTarget(root, relative string) (string, string, error) {
	relative, target, err := validateUploadTargetPath(root, relative)
	if err != nil {
		return "", "", err
	}
	if _, err := os.Lstat(target); err == nil {
		return "", "", ErrUploadTargetExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", err
	}
	return relative, target, nil
}

func validateUploadTargetPath(root, relative string) (string, string, error) {
	parentRelative, name, err := splitTarget(relative)
	if err != nil {
		return "", "", err
	}
	parent, err := resolveExisting(root, parentRelative)
	if err != nil {
		return "", "", err
	}
	info, err := os.Lstat(parent)
	if err != nil {
		return "", "", err
	}
	if !info.IsDir() {
		return "", "", errors.New("upload parent is not a directory")
	}
	relative = cleanRelative(relative)
	return relative, filepath.Join(parent, name), nil
}

func normalizeSHA256(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "", nil
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha256.Size {
		return "", errors.New("SHA-256 must be 64 hexadecimal characters")
	}
	return value, nil
}

func newUploadID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func validUploadID(id string) bool {
	if len(id) != 32 {
		return false
	}
	for _, r := range id {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func uploadMetadataPath(root, id string) string {
	return filepath.Join(root, uploadPrefix+id+uploadMetadataSuffix)
}

func uploadPartPath(root, id string) string {
	return filepath.Join(root, uploadPrefix+id+uploadPartSuffix)
}

func writeUploadMetadata(root string, session UploadSession) error {
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temp, err := os.CreateTemp(root, uploadPrefix+"metadata-*")
	if err != nil {
		return fmt.Errorf("create upload metadata temp: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o640); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
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
	if err := os.Rename(tempPath, uploadMetadataPath(root, session.ID)); err != nil {
		return fmt.Errorf("replace upload metadata: %w", err)
	}
	return nil
}
