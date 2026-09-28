package security

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	passwordMinBytes = 12
	passwordMaxBytes = 1024
	argonMemory      = uint32(19 * 1024)
	argonTime        = uint32(2)
	argonThreads     = uint8(1)
	argonSaltLength  = 16
	argonKeyLength   = uint32(32)
)

func ValidatePassword(password string) error {
	size := len([]byte(password))
	if size < passwordMinBytes {
		return fmt.Errorf("password must be at least %d bytes", passwordMinBytes)
	}
	if size > passwordMaxBytes {
		return fmt.Errorf("password must be at most %d bytes", passwordMaxBytes)
	}
	return nil
}

func HashPassword(password string) (string, error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}

	salt := make([]byte, argonSaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}

	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLength)
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		argonMemory,
		argonTime,
		argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

func VerifyPassword(encoded, password string) (bool, error) {
	params, salt, expected, err := parsePasswordHash(encoded)
	if err != nil {
		return false, err
	}
	actual := argon2.IDKey([]byte(password), salt, params.time, params.memory, params.threads, uint32(len(expected)))
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}

type passwordParams struct {
	memory  uint32
	time    uint32
	threads uint8
}

func parsePasswordHash(encoded string) (passwordParams, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return passwordParams{}, nil, nil, errors.New("unsupported password hash format")
	}
	if parts[2] != "v="+strconv.Itoa(argon2.Version) {
		return passwordParams{}, nil, nil, errors.New("unsupported argon2 version")
	}

	var memory, iterations uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &threads); err != nil {
		return passwordParams{}, nil, nil, errors.New("invalid argon2 parameters")
	}
	if memory < 7*1024 || memory > 256*1024 || iterations < 1 || iterations > 10 || threads < 1 || threads > 8 {
		return passwordParams{}, nil, nil, errors.New("argon2 parameters outside allowed bounds")
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) < 8 || len(salt) > 64 {
		return passwordParams{}, nil, nil, errors.New("invalid password salt")
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(expected) < 16 || len(expected) > 64 {
		return passwordParams{}, nil, nil, errors.New("invalid password digest")
	}

	return passwordParams{memory: memory, time: iterations, threads: threads}, salt, expected, nil
}
