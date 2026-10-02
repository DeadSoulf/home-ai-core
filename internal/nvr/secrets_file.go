package nvr

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	credentialDirName = "nvr-secrets"
	credentialKeyName = "key"
)

type encryptedCredentialStore struct {
	dir  string
	aead cipher.AEAD
}

type storedCameraCredential struct {
	CameraID string `json:"camera_id"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func NewFileCredentialStore(stateDir string) (CameraCredentialStore, error) {
	stateDir = strings.TrimSpace(stateDir)
	if stateDir == "" {
		return nil, errors.New("NVR credential state directory is required")
	}
	dir := filepath.Join(stateDir, credentialDirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create NVR credential directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("protect NVR credential directory: %w", err)
	}
	key, err := loadOrCreateCredentialKey(filepath.Join(dir, credentialKeyName))
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("initialize NVR credential cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("initialize NVR credential AEAD: %w", err)
	}
	return &encryptedCredentialStore{dir: dir, aead: aead}, nil
}

func loadOrCreateCredentialKey(path string) ([]byte, error) {
	key, err := os.ReadFile(path)
	if err == nil {
		if len(key) != 32 {
			return nil, errors.New("NVR credential key has invalid length")
		}
		if err := os.Chmod(path, 0o600); err != nil {
			return nil, fmt.Errorf("protect NVR credential key: %w", err)
		}
		return key, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read NVR credential key: %w", err)
	}
	key = make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("generate NVR credential key: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		existing, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, fmt.Errorf("read concurrently created NVR credential key: %w", readErr)
		}
		if len(existing) != 32 {
			return nil, errors.New("NVR credential key has invalid length")
		}
		return existing, nil
	}
	if err != nil {
		return nil, fmt.Errorf("create NVR credential key: %w", err)
	}
	defer file.Close()
	if _, err := file.Write(key); err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("write NVR credential key: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("sync NVR credential key: %w", err)
	}
	return key, nil
}

func (s *encryptedCredentialStore) PutCameraCredential(
	ctx context.Context,
	cameraID string,
	credential CameraCredential,
) (SecretRef, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	cameraID = strings.TrimSpace(cameraID)
	credential.Username = strings.TrimSpace(credential.Username)
	if cameraID == "" {
		return "", errors.New("camera id is required for credential storage")
	}
	if credential.Username == "" && credential.Password == "" {
		return "", nil
	}
	ref, err := newSecretRef()
	if err != nil {
		return "", err
	}
	plain, err := json.Marshal(storedCameraCredential{
		CameraID: cameraID,
		Username: credential.Username,
		Password: credential.Password,
	})
	if err != nil {
		return "", fmt.Errorf("encode camera credential: %w", err)
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate camera credential nonce: %w", err)
	}
	sealed := s.aead.Seal(nil, nonce, plain, []byte(ref))
	payload := append(nonce, sealed...)
	target := s.path(ref)
	tmp, err := os.CreateTemp(s.dir, ".credential-*")
	if err != nil {
		return "", fmt.Errorf("create temporary camera credential: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("protect temporary camera credential: %w", err)
	}
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("write camera credential: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("sync camera credential: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("close camera credential: %w", err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		return "", fmt.Errorf("commit camera credential: %w", err)
	}
	if err := os.Chmod(target, 0o600); err != nil {
		return "", fmt.Errorf("protect camera credential: %w", err)
	}
	return ref, nil
}

func (s *encryptedCredentialStore) ResolveCameraCredential(
	ctx context.Context,
	ref SecretRef,
) (CameraCredential, error) {
	if err := ctx.Err(); err != nil {
		return CameraCredential{}, err
	}
	if _, err := ParseSecretRef(string(ref)); err != nil {
		return CameraCredential{}, err
	}
	payload, err := os.ReadFile(s.path(ref))
	if errors.Is(err, os.ErrNotExist) {
		return CameraCredential{}, ErrSecretStoreUnavailable
	}
	if err != nil {
		return CameraCredential{}, fmt.Errorf("read camera credential: %w", err)
	}
	nonceSize := s.aead.NonceSize()
	if len(payload) <= nonceSize {
		return CameraCredential{}, errors.New("camera credential payload is invalid")
	}
	plain, err := s.aead.Open(nil, payload[:nonceSize], payload[nonceSize:], []byte(ref))
	if err != nil {
		return CameraCredential{}, errors.New("camera credential cannot be decrypted")
	}
	var stored storedCameraCredential
	if err := json.Unmarshal(plain, &stored); err != nil {
		return CameraCredential{}, errors.New("camera credential payload is invalid")
	}
	return CameraCredential{Username: stored.Username, Password: stored.Password}, nil
}

func (s *encryptedCredentialStore) DeleteCameraCredential(ctx context.Context, ref SecretRef) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if ref == "" {
		return nil
	}
	if _, err := ParseSecretRef(string(ref)); err != nil {
		return err
	}
	err := os.Remove(s.path(ref))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("delete camera credential: %w", err)
	}
	return nil
}

func (s *encryptedCredentialStore) path(ref SecretRef) string {
	return filepath.Join(s.dir, string(ref)+".bin")
}

func newSecretRef() (SecretRef, error) {
	raw := make([]byte, 18)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return "", fmt.Errorf("generate camera secret reference: %w", err)
	}
	return SecretRef(SecretRefPrefix + base64.RawURLEncoding.EncodeToString(raw)), nil
}
