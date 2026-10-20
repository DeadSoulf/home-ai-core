package config

import (
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
)

const (
	defaultListen   = "0.0.0.0:82"
	defaultStateDir = "/var/lib/home-ai-core"
	defaultWebDir   = "/usr/share/home-ai-core/web"
)

type Config struct {
	ListenAddress    string
	FrigateUIListen  string
	StateDir         string
	WebDir           string
	TLSCertFile      string
	TLSKeyFile       string
	InsecureLoopback bool
}

func Load(args []string) (Config, error) {
	cfg := Config{
		ListenAddress:   envOrDefault("HOME_AI_LISTEN", defaultListen),
		FrigateUIListen: strings.TrimSpace(os.Getenv("HOME_AI_FRIGATE_UI_LISTEN")),
		StateDir:        envOrDefault("HOME_AI_STATE_DIR", defaultStateDir),
		WebDir:          envOrDefault("HOME_AI_WEB_DIR", defaultWebDir),
		TLSCertFile:     strings.TrimSpace(os.Getenv("HOME_AI_TLS_CERT")),
		TLSKeyFile:      strings.TrimSpace(os.Getenv("HOME_AI_TLS_KEY")),
	}
	fs := flag.NewFlagSet("home-ai-core", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&cfg.ListenAddress, "listen", cfg.ListenAddress, "HTTPS listen address")
	fs.StringVar(&cfg.FrigateUIListen, "frigate-ui-listen", cfg.FrigateUIListen, "isolated Frigate HTTPS listener (default same interface:8971; off disables)")
	fs.StringVar(&cfg.StateDir, "state-dir", cfg.StateDir, "persistent state directory")
	fs.StringVar(&cfg.WebDir, "web-dir", cfg.WebDir, "built Web UI directory")
	fs.StringVar(&cfg.TLSCertFile, "tls-cert", cfg.TLSCertFile, "PEM certificate chain (optional)")
	fs.StringVar(&cfg.TLSKeyFile, "tls-key", cfg.TLSKeyFile, "PEM private key (optional)")
	fs.BoolVar(&cfg.InsecureLoopback, "allow-insecure-loopback", false, "development-only HTTP on a literal loopback IP")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	if fs.NArg() != 0 {
		return Config{}, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if strings.TrimSpace(cfg.StateDir) == "" || strings.TrimSpace(cfg.WebDir) == "" {
		return Config{}, fmt.Errorf("state and web directories must not be empty")
	}
	host, port, err := net.SplitHostPort(cfg.ListenAddress)
	if err != nil {
		return Config{}, fmt.Errorf("invalid listen address: %w", err)
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 0 || n > 65535 {
		return Config{}, fmt.Errorf("invalid listen port")
	}
	if (cfg.TLSCertFile == "") != (cfg.TLSKeyFile == "") {
		return Config{}, fmt.Errorf("TLS certificate and key must be configured together")
	}
	if cfg.InsecureLoopback {
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return Config{}, fmt.Errorf("plaintext HTTP is permitted only on a literal loopback address")
		}
	}
	if cfg.FrigateUIListen == "" {
		cfg.FrigateUIListen = net.JoinHostPort(host, "8971")
	}
	if cfg.FrigateUIListen != "off" {
		uiHost, uiPort, e := net.SplitHostPort(cfg.FrigateUIListen)
		uiN, pe := strconv.Atoi(uiPort)
		if e != nil || pe != nil || uiN < 1 || uiN > 65535 || uiN == n || uiHost != host {
			return Config{}, fmt.Errorf("Frigate UI must use the same bind interface and a separate valid port, or off")
		}
	}
	return cfg, nil
}

func envOrDefault(name, fallback string) string {
	if value, ok := os.LookupEnv(name); ok && strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}
