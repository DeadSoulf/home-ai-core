package config

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	defaultListen   = "0.0.0.0:8080"
	defaultStateDir = "/var/lib/home-ai-core"
	defaultWebDir   = "/usr/share/home-ai-core/web"
)

type Config struct {
	ListenAddress   string
	StateDir        string
	WebDir          string
}

func Load(args []string) (Config, error) {
	cfg := Config{
		ListenAddress:   envOrDefault("HOME_AI_LISTEN", defaultListen),
		StateDir:        envOrDefault("HOME_AI_STATE_DIR", defaultStateDir),
		WebDir:          envOrDefault("HOME_AI_WEB_DIR", defaultWebDir),
	}

	fs := flag.NewFlagSet("home-ai-core", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&cfg.ListenAddress, "listen", cfg.ListenAddress, "HTTP listen address")
	fs.StringVar(&cfg.StateDir, "state-dir", cfg.StateDir, "persistent state directory")
	fs.StringVar(&cfg.WebDir, "web-dir", cfg.WebDir, "built Web UI directory")

	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	if fs.NArg() != 0 {
		return Config{}, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if strings.TrimSpace(cfg.ListenAddress) == "" {
		return Config{}, fmt.Errorf("listen address must not be empty")
	}
	if strings.TrimSpace(cfg.StateDir) == "" {
		return Config{}, fmt.Errorf("state directory must not be empty")
	}
	if strings.TrimSpace(cfg.WebDir) == "" {
		return Config{}, fmt.Errorf("web directory must not be empty")
	}

	return cfg, nil
}

func envOrDefault(name, fallback string) string {
	if value, ok := os.LookupEnv(name); ok && strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}
