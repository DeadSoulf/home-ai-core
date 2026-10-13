package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
)

// VerifySignedChecksum binds the publisher's signature to an exact archive
// name and digest. Used again by the privileged helper, not just by Core.
func VerifySignedChecksum(checksum, signature, key []byte, expectedFile string) (string, error) {
	if len(checksum) > 4096 || len(signature) > 4096 || len(key) > 4096 {
		return "", errors.New("update trust metadata exceeds limit")
	}
	if err := verifyChecksumSignature(checksum, signature, key); err != nil {
		return "", err
	}
	fields := strings.Fields(string(checksum))
	if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != expectedFile {
		return "", errors.New("signed checksum archive name mismatch")
	}
	digest := strings.ToLower(fields[0])
	if len(digest) != sha256.Size*2 {
		return "", errors.New("invalid signed archive digest")
	}
	if _, err := hex.DecodeString(digest); err != nil {
		return "", err
	}
	return digest, nil
}

// ExtractVerifiedArchive must only receive a caller-owned, immutable archive
// whose entire digest was verified against VerifySignedChecksum.
func ExtractVerifiedArchive(archive, target, version, architecture string) error {
	return extractAndVerifyBundle(archive, target, version, architecture)
}
