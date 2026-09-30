package windowsclient

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrCredentialStoreUnsupported = errors.New("Windows Credential Manager is unavailable on this platform")
	ErrCredentialNotFound         = errors.New("stored Home-AI credential not found")
)

func credentialTarget(server, username string) (target string, normalizedServer string, normalizedUsername string, err error) {
	normalizedServer, err = normalizeQueueServer(server)
	if err != nil {
		return "", "", "", err
	}
	normalizedUsername = strings.TrimSpace(username)
	if normalizedUsername == "" || normalizedUsername != username {
		return "", "", "", errors.New("username is required and must not contain surrounding whitespace")
	}
	sum := sha256.Sum256([]byte(normalizedServer + "\x00" + normalizedUsername))
	target = "HomeAI.WindowsClient." + hex.EncodeToString(sum[:])
	return target, normalizedServer, normalizedUsername, nil
}

func CredentialTarget(server, username string) (string, error) {
	target, _, _, err := credentialTarget(server, username)
	return target, err
}

func validateCredentialPassword(password string) error {
	if password == "" {
		return errors.New("password cannot be empty")
	}
	if len([]byte(password)) > 5*512 {
		return fmt.Errorf("password exceeds Windows Credential Manager generic credential limit")
	}
	return nil
}
