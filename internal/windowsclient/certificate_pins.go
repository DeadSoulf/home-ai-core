package windowsclient

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const serverPinStoreVersion = 1
const serverPinStoreMaxBytes = 64 << 10
const maxPinnedServers = 128

type serverPinStore struct {
	Version int `json:"version"`
	Servers map[string]string `json:"servers"`
}

// ServerCertificateInfo is public server certificate metadata. Probing sends
// no Home-AI credentials or requests; it must never authenticate the server.
type ServerCertificateInfo struct {
	SHA256 string
	Subject string
	Issuer string
	NotBefore time.Time
	NotAfter time.Time
}

func pinnedServerFile() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user configuration: %w", err)
	}
	return filepath.Join(base, "HomeAI", "trusted-servers.json"), nil
}

// serverPinOrigin normalizes the exact TLS origin without calling New. New is
// used by NormalizeServerURL, so calling NormalizeServerURL here would recurse.
func serverPinOrigin(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("parse server origin: %w", err)
	}
	if parsed.Scheme != "https" || parsed.Hostname() == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		parsed.Opaque != "" || parsed.ForceQuery {
		return "", errors.New("certificate pin requires a clean HTTPS server URL")
	}
	host := strings.ToLower(parsed.Hostname())
	port := parsed.Port()
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return "", errors.New("invalid HTTPS port")
		}
	}
	if port == "" || port == "443" {
		if strings.Contains(host, ":") {
			return "https://[" + host + "]", nil
		}
		return "https://" + host, nil
	}
	return "https://" + net.JoinHostPort(host, port), nil
}

func normalizedFingerprint(fingerprint string) (string, error) {
	pin := strings.ToLower(strings.TrimSpace(fingerprint))
	if len(pin) != sha256.Size * 2 {
		return "", errors.New("server certificate SHA-256 must have 64 hex characters")
	}
	b, err := hex.DecodeString(pin)
	if err != nil || len(b) != sha256.Size {
		return "", errors.New("invalid SHA-256 certificate fingerprint")
	}
	return pin, nil
}

func readServerPinStore(filename string) (serverPinStore, error) {
	file, err := os.Open(filename)
	if errors.Is(err, os.ErrNotExist) {
		return serverPinStore{Version: serverPinStoreVersion, Servers: make(map[string]string)}, nil
	}
	if err != nil {
		return serverPinStore{}, fmt.Errorf("open trusted server pins: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return serverPinStore{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > serverPinStoreMaxBytes {
		return serverPinStore{}, errors.New("trusted server pin store is invalid or too large")
	}
	var store serverPinStore
	decoder := json.NewDecoder(io.LimitReader(file, serverPinStoreMaxBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&store); err != nil {
		return store, fmt.Errorf("decode trusted server pins: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return store, errors.New("trusted server pin store has extra data")
	}
	if store.Version != serverPinStoreVersion || store.Servers == nil || len(store.Servers) > maxPinnedServers {
		return store, errors.New("invalid trusted server pin store version or size")
	}
	for origin, fingerprint := range store.Servers {
		canonical, err := serverPinOrigin(origin)
		if err != nil || canonical != origin {
			return store, errors.New("invalid trusted server origin")
		}
		normalized, err := normalizedFingerprint(fingerprint)
		if err != nil || normalized != fingerprint {
			return store, errors.New("invalid trusted server fingerprint")
		}
	}
	return store, nil
}

func loadServerPinAt(filename, rawServer string) (string, error) {
	origin, err := serverPinOrigin(rawServer)
	if err != nil {
		return "", err
	}
	store, err := readServerPinStore(filename)
	if err != nil {
		return "", err
	}
	return store.Servers[origin], nil
}

func LoadServerCertificatePin(rawServer string) (string, error) {
	filename, err := pinnedServerFile()
	if err != nil {
		return "", err
	}
	return loadServerPinAt(filename, rawServer)
}

func saveServerPinAt(filename, rawServer, fingerprint string) error {
	origin, err := serverPinOrigin(rawServer)
	if err != nil {
		return err
	}
	if fingerprint != "" {
		fingerprint, err = normalizedFingerprint(fingerprint)
		if err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
		return err
	}
	lock, err := lockQueueFile(filename + ".lock")
	if err != nil {
		return fmt.Errorf("lock trusted server pins: %w", err)
	}
	defer func() {
		_ = unlockQueueFile(lock)
		_ = lock.Close()
	}()
	store, err := readServerPinStore(filename)
	if err != nil {
		return err
	}
	if fingerprint != "" {
		store.Servers[origin] = fingerprint
	} else {
		delete(store.Servers, origin)
	}
	if len(store.Servers) > maxPinnedServers {
		return errors.New("too many pinned servers")
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	if len(data) > serverPinStoreMaxBytes {
		return errors.New("trusted server pin store is too large")
	}
	temporary, err := os.CreateTemp(filepath.Dir(filename), ".home-ai-server-pins-*")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err = temporary.Write(append(data, '\n')); err != nil {
		temporary.Close()
		return err
	}
	if err = temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err = temporary.Close(); err != nil {
		return err
	}
	return replaceQueueFile(temporary.Name(), filename)
}

// SaveServerCertificatePin must only be called after the user has compared the
// SHA-256 fingerprint against an independently trusted server-console value.
func SaveServerCertificatePin(rawServer, fingerprint string) error {
	filename, err := pinnedServerFile()
	if err != nil {
		return err
	}
	return saveServerPinAt(filename, rawServer, fingerprint)
}

func RemoveServerCertificatePin(rawServer string) error {
	filename, err := pinnedServerFile()
	if err != nil {
		return err
	}
	return saveServerPinAt(filename, rawServer, "")
}

func verifyPinnedCertificate(connection tls.ConnectionState, expected [32]byte) error {
	if len(connection.PeerCertificates) == 0 {
		return errors.New("TLS server did not provide a certificate")
	}
	leaf := connection.PeerCertificates[0]
	received := sha256.Sum256(leaf.Raw)
	if subtle.ConstantTimeCompare(received[:], expected[:]) != 1 {
		return errors.New("server TLS certificate has changed; re-verify its fingerprint on the server before trusting it")
	}
	now := time.Now()
	if now.Before(leaf.NotBefore) || !now.Before(leaf.NotAfter) {
		return errors.New("pinned TLS certificate is not currently valid")
	}
	if len(leaf.ExtKeyUsage) > 0 {
		allowed := false
		for _, usage := range leaf.ExtKeyUsage {
			if usage == x509.ExtKeyUsageServerAuth || usage == x509.ExtKeyUsageAny {
				allowed = true
			}
		}
		if !allowed {
			return errors.New("pinned TLS certificate is not valid for server authentication")
		}
	}
	return nil
}

// pinnedTLSConfig intentionally replaces CA/hostname checks only with a
// previously verified exact leaf-certificate digest. VerifyConnection runs for
// every handshake even when InsecureSkipVerify is true. Do not use this mode
// without a pin, and never auto-approve fingerprints received over the network.
func pinnedTLSConfig(serverName, fingerprint string) (*tls.Config, error) {
	pin, err := normalizedFingerprint(fingerprint)
	if err != nil {
		return nil, err
	}
	raw, _ := hex.DecodeString(pin)
	var expected [32]byte
	copy(expected[:], raw)
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		ServerName: serverName,
		InsecureSkipVerify: true,
		VerifyConnection: func(state tls.ConnectionState) error {
			return verifyPinnedCertificate(state, expected)
		},
	}, nil
}

// ProbeServerCertificate deliberately performs an unauthenticated TLS probe;
// the result is untrusted until its digest is verified through a separate
// channel. It never sends a login request, password, cookie or bearer token.
func ProbeServerCertificate(ctx context.Context, rawServer string) (ServerCertificateInfo, error) {
	origin, err := serverPinOrigin(rawServer)
	if err != nil {
		return ServerCertificateInfo{}, err
	}
	parsed, _ := url.Parse(origin)
	port := parsed.Port()
	if port == "" {
		port = "443"
	}
	var dialer net.Dialer
	connection, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(parsed.Hostname(), port))
	if err != nil {
		return ServerCertificateInfo{}, err
	}
	defer connection.Close()
	tlsConnection := tls.Client(connection, &tls.Config{
		ServerName: parsed.Hostname(), MinVersion: tls.VersionTLS12,
		InsecureSkipVerify: true, // Unauthenticated inspection only; never used to send credentials.
	})
	defer tlsConnection.Close()
	if err := tlsConnection.HandshakeContext(ctx); err != nil {
		return ServerCertificateInfo{}, err
	}
	certs := tlsConnection.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return ServerCertificateInfo{}, errors.New("server did not present a TLS certificate")
	}
	leaf := certs[0]
	if time.Now().Before(leaf.NotBefore) || !time.Now().Before(leaf.NotAfter) {
		return ServerCertificateInfo{}, errors.New("server certificate is expired or not yet valid")
	}
	sum := sha256.Sum256(leaf.Raw)
	return ServerCertificateInfo{
		SHA256: hex.EncodeToString(sum[:]), Subject: leaf.Subject.String(),
		Issuer: leaf.Issuer.String(), NotBefore: leaf.NotBefore, NotAfter: leaf.NotAfter,
	}, nil
}
