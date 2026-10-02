package nvr

import (
	"context"
	"errors"
	"strings"
	"unicode"
)

const SecretRefPrefix = "sec_"

var ErrSecretStoreUnavailable = errors.New("NVR secret store is not available")

type SecretRef string

func ParseSecretRef(value string) (SecretRef, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if len(value) < 5 || len(value) > 128 || !strings.HasPrefix(value, SecretRefPrefix) {
		return "", errors.New("secret reference is invalid")
	}
	for _, r := range strings.TrimPrefix(value, SecretRefPrefix) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			continue
		}
		return "", errors.New("secret reference contains unsupported characters")
	}
	return SecretRef(value), nil
}

type CameraCredential struct {
	Username string `json:"-"`
	Password string `json:"-"`
}

type CameraCredentialStore interface {
	PutCameraCredential(context.Context, string, CameraCredential) (SecretRef, error)
	ResolveCameraCredential(context.Context, SecretRef) (CameraCredential, error)
	DeleteCameraCredential(context.Context, SecretRef) error
}
