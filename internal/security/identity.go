package security

import (
	"fmt"
	"strings"
)

func NormalizeUsername(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) < 3 || len(value) > 64 {
		return "", fmt.Errorf("username must contain 3 to 64 characters")
	}

	for i, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case (r == '.' || r == '_' || r == '-') && i > 0:
		default:
			return "", fmt.Errorf("username contains unsupported characters")
		}
	}
	return value, nil
}

func NormalizeDisplayName(value, fallback string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	}
	if len([]rune(value)) > 128 {
		return "", fmt.Errorf("display name is too long")
	}
	return value, nil
}
