package security

import (
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const bootstrapFile = "bootstrap-token"

func (s *Service) prepareBootstrap(initialized bool) error {
	path := filepath.Join(s.stateDir, bootstrapFile)
	if initialized {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove stale bootstrap token: %w", err)
		}
		return nil
	}

	if data, err := os.ReadFile(path); err == nil {
		if validBootstrapToken(strings.TrimSpace(string(data))) {
			return nil
		}
		return fmt.Errorf("invalid existing bootstrap token file")
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read bootstrap token: %w", err)
	}

	token, err := newSecret()
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create bootstrap token: %w", err)
	}
	defer file.Close()

	if _, err := file.WriteString(token + "\n"); err != nil {
		return fmt.Errorf("write bootstrap token: %w", err)
	}
	return file.Sync()
}

func (s *Service) verifyBootstrapToken(candidate string) bool {
	data, err := os.ReadFile(filepath.Join(s.stateDir, bootstrapFile))
	if err != nil {
		return false
	}
	expected := strings.TrimSpace(string(data))
	if !validBootstrapToken(candidate) || !validBootstrapToken(expected) {
		return false
	}
	expectedHash := hashSecret(expected)
	candidateHash := hashSecret(candidate)
	return subtle.ConstantTimeCompare([]byte(expectedHash), []byte(candidateHash)) == 1
}

func (s *Service) removeBootstrapToken() error {
	err := os.Remove(filepath.Join(s.stateDir, bootstrapFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func validBootstrapToken(value string) bool {
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(value))
	return err == nil && len(raw) == 32
}

func (s *Service) BootstrapTokenPath() string {
	return filepath.Join(s.stateDir, bootstrapFile)
}
