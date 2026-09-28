package identity

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const nodeIDFile = "node-id"

func LoadOrCreate(stateDir string) (string, error) {
	identityDir := filepath.Join(stateDir, "identity")
	path := filepath.Join(identityDir, nodeIDFile)

	if id, err := read(path); err == nil {
		return id, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	if err := os.MkdirAll(identityDir, 0o700); err != nil {
		return "", fmt.Errorf("create identity directory: %w", err)
	}

	id, err := newUUID()
	if err != nil {
		return "", err
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return read(path)
		}
		return "", fmt.Errorf("create node identity: %w", err)
	}

	ok := false
	defer func() {
		_ = file.Close()
		if !ok {
			_ = os.Remove(path)
		}
	}()

	if _, err := file.WriteString(id + "\n"); err != nil {
		return "", fmt.Errorf("write node identity: %w", err)
	}
	if err := file.Sync(); err != nil {
		return "", fmt.Errorf("sync node identity: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close node identity: %w", err)
	}

	ok = true
	return id, nil
}

func read(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(string(data))
	if !validUUID(id) {
		return "", fmt.Errorf("invalid node identity in %s", path)
	}
	return id, nil
}

func newUUID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate node identity: %w", err)
	}

	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80

	buf := make([]byte, 36)
	hex.Encode(buf[0:8], raw[0:4])
	buf[8] = '-'
	hex.Encode(buf[9:13], raw[4:6])
	buf[13] = '-'
	hex.Encode(buf[14:18], raw[6:8])
	buf[18] = '-'
	hex.Encode(buf[19:23], raw[8:10])
	buf[23] = '-'
	hex.Encode(buf[24:36], raw[10:16])

	return string(buf), nil
}

func validUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for _, pos := range []int{8, 13, 18, 23} {
		if value[pos] != '-' {
			return false
		}
	}
	for i, r := range value {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			continue
		}
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}
