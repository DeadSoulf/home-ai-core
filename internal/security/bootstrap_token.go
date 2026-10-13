package security

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var ErrInvalidBootstrapToken = errors.New("a valid one-time setup code from the server console is required")

// initializeBootstrapToken exposes the setup secret only as a mode-0600 file
// in the private state directory. It is never returned by a Web/API endpoint.
func (s *Service) initializeBootstrapToken(ctx context.Context, stateDir string) error {
	initialized, err := s.Initialized(ctx)
	if err != nil {
		return err
	}
	s.bootstrapTokenPath = filepath.Join(stateDir, "bootstrap-token")
	if initialized {
		_ = os.Remove(s.bootstrapTokenPath)
		return nil
	}
	info, err := os.Lstat(s.bootstrapTokenPath)
	if errors.Is(err, os.ErrNotExist) {
		token, err := newSecret()
		if err != nil {
			return err
		}
		f, err := os.OpenFile(s.bootstrapTokenPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		_, we := f.WriteString(token + "\n")
		se := f.Sync()
		ce := f.Close()
		if err := errors.Join(we, se, ce); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return errors.New("setup token must be a private regular file (mode 0600)")
	}
	data, err := os.ReadFile(s.bootstrapTokenPath)
	if err != nil {
		return err
	}
	value := strings.TrimSpace(string(data))
	if len(value) < 32 || len(value) > 128 {
		return fmt.Errorf("invalid setup token file; regenerate it from the server console")
	}
	s.bootstrapTokenHash = hashSecret(value)
	return nil
}

func (s *Service) CheckBootstrapToken(ctx context.Context, value string) error {
	initialized, err := s.Initialized(ctx)
	if err != nil {
		return err
	}
	if initialized {
		return ErrAlreadyInitialized
	}
	if s.bootstrapTokenHash == "" || len(value) > 128 || subtle.ConstantTimeCompare([]byte(s.bootstrapTokenHash), []byte(hashSecret(strings.TrimSpace(value)))) != 1 {
		return ErrInvalidBootstrapToken
	}
	return nil
}
