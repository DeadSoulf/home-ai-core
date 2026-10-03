package nvr

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

type httpDigestChallenge struct {
	Realm     string
	Nonce     string
	Opaque    string
	Algorithm string
	QOP       string
}

func parseHTTPDigestChallenge(value string) (httpDigestChallenge, error) {
	value = strings.TrimSpace(value)
	if len(value) < len("Digest ") || !strings.EqualFold(value[:len("Digest ")], "Digest ") {
		return httpDigestChallenge{}, errors.New("HTTP Digest challenge missing")
	}
	params := parseHTTPAuthParams(strings.TrimSpace(value[len("Digest "):]))
	challenge := httpDigestChallenge{
		Realm:     params["realm"],
		Nonce:     params["nonce"],
		Opaque:    params["opaque"],
		Algorithm: strings.TrimSpace(params["algorithm"]),
	}
	if challenge.Realm == "" || challenge.Nonce == "" {
		return httpDigestChallenge{}, errors.New("HTTP Digest challenge is incomplete")
	}
	if challenge.Algorithm == "" {
		challenge.Algorithm = "MD5"
	}

	qop := strings.TrimSpace(params["qop"])
	if qop != "" {
		for _, candidate := range strings.Split(qop, ",") {
			if strings.EqualFold(strings.TrimSpace(candidate), "auth") {
				challenge.QOP = "auth"
				break
			}
		}
		if challenge.QOP == "" {
			return httpDigestChallenge{}, errors.New("HTTP Digest qop is unsupported")
		}
	}
	return challenge, nil
}

func parseHTTPAuthParams(value string) map[string]string {
	result := make(map[string]string)
	for len(value) > 0 {
		value = strings.TrimLeft(value, " ,\t")
		if value == "" {
			break
		}
		eq := strings.IndexByte(value, '=')
		if eq <= 0 {
			break
		}
		key := strings.ToLower(strings.TrimSpace(value[:eq]))
		value = strings.TrimSpace(value[eq+1:])
		if value == "" {
			result[key] = ""
			break
		}

		var parsed string
		if value[0] == '"' {
			var builder strings.Builder
			escaped := false
			index := 1
			for ; index < len(value); index++ {
				ch := value[index]
				switch {
				case escaped:
					builder.WriteByte(ch)
					escaped = false
				case ch == '\\':
					escaped = true
				case ch == '"':
					index++
					goto quotedDone
				default:
					builder.WriteByte(ch)
				}
			}
		quotedDone:
			parsed = builder.String()
			value = value[index:]
		} else {
			index := strings.IndexByte(value, ',')
			if index < 0 {
				parsed = strings.TrimSpace(value)
				value = ""
			} else {
				parsed = strings.TrimSpace(value[:index])
				value = value[index+1:]
			}
		}
		result[key] = parsed
	}
	return result
}

func buildHTTPDigestAuthorization(
	challengeValue string,
	username string,
	password string,
	method string,
	uri string,
) (string, error) {
	challenge, err := parseHTTPDigestChallenge(challengeValue)
	if err != nil {
		return "", err
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return "", errors.New("HTTP Digest username is required")
	}

	algorithm := strings.ToUpper(strings.TrimSpace(challenge.Algorithm))
	sess := strings.HasSuffix(algorithm, "-SESS")
	baseAlgorithm := strings.TrimSuffix(algorithm, "-SESS")
	if baseAlgorithm != "MD5" && baseAlgorithm != "SHA-256" {
		return "", fmt.Errorf("HTTP Digest algorithm %s is unsupported", challenge.Algorithm)
	}

	cnonceRaw := make([]byte, 16)
	if _, err := rand.Read(cnonceRaw); err != nil {
		return "", fmt.Errorf("generate HTTP Digest cnonce: %w", err)
	}
	cnonce := hex.EncodeToString(cnonceRaw)
	nc := "00000001"

	hash := func(value string) string {
		if baseAlgorithm == "SHA-256" {
			sum := sha256.Sum256([]byte(value))
			return hex.EncodeToString(sum[:])
		}
		sum := md5.Sum([]byte(value))
		return hex.EncodeToString(sum[:])
	}

	ha1 := hash(username + ":" + challenge.Realm + ":" + password)
	if sess {
		ha1 = hash(ha1 + ":" + challenge.Nonce + ":" + cnonce)
	}
	ha2 := hash(strings.ToUpper(method) + ":" + uri)

	var response string
	if challenge.QOP != "" {
		response = hash(
			ha1 + ":" + challenge.Nonce + ":" + nc + ":" + cnonce + ":" + challenge.QOP + ":" + ha2,
		)
	} else {
		response = hash(ha1 + ":" + challenge.Nonce + ":" + ha2)
	}

	parts := []string{
		"username=\"" + escapeHTTPDigestValue(username) + "\"",
		"realm=\"" + escapeHTTPDigestValue(challenge.Realm) + "\"",
		"nonce=\"" + escapeHTTPDigestValue(challenge.Nonce) + "\"",
		"uri=\"" + escapeHTTPDigestValue(uri) + "\"",
		"response=\"" + response + "\"",
		"algorithm=" + challenge.Algorithm,
	}
	if challenge.Opaque != "" {
		parts = append(parts, "opaque=\""+escapeHTTPDigestValue(challenge.Opaque)+"\"")
	}
	if challenge.QOP != "" {
		parts = append(
			parts,
			"qop="+challenge.QOP,
			"nc="+nc,
			"cnonce=\""+cnonce+"\"",
		)
	}
	return "Digest " + strings.Join(parts, ", "), nil
}

func escapeHTTPDigestValue(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	return strings.ReplaceAll(value, "\"", "\\\"")
}
