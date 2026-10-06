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
	AIProvider      string
	AIEndpoint      string
	AIModel         string
	CloudAIEndpoint string
	CloudAIModel    string
	CloudAIAPIKey   string
}

func Load(args []string) (Config, error) {
	cfg := Config{
		ListenAddress:   envOrDefault("HOME_AI_LISTEN", defaultListen),
		StateDir:        envOrDefault("HOME_AI_STATE_DIR", defaultStateDir),
		WebDir:          envOrDefault("HOME_AI_WEB_DIR", defaultWebDir),
		AIProvider:      strings.ToLower(strings.TrimSpace(os.Getenv("HOME_AI_AI_PROVIDER"))),
		AIEndpoint:      strings.TrimSpace(os.Getenv("HOME_AI_AI_ENDPOINT")),
		AIModel:         strings.TrimSpace(os.Getenv("HOME_AI_AI_MODEL")),
		CloudAIEndpoint: strings.TrimSpace(os.Getenv("HOME_AI_CLOUD_AI_ENDPOINT")),
		CloudAIModel:    strings.TrimSpace(os.Getenv("HOME_AI_CLOUD_AI_MODEL")),
		CloudAIAPIKey:   strings.TrimSpace(os.Getenv("HOME_AI_CLOUD_AI_API_KEY")),
	}

	fs := flag.NewFlagSet("home-ai-core", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&cfg.ListenAddress, "listen", cfg.ListenAddress, "HTTP listen address")
	fs.StringVar(&cfg.StateDir, "state-dir", cfg.StateDir, "persistent state directory")
	fs.StringVar(&cfg.WebDir, "web-dir", cfg.WebDir, "built Web UI directory")
	fs.StringVar(&cfg.AIProvider, "ai-provider", cfg.AIProvider, "AI provider (empty or ollama)")
	fs.StringVar(&cfg.AIEndpoint, "ai-endpoint", cfg.AIEndpoint, "AI provider endpoint")
	fs.StringVar(&cfg.AIModel, "ai-model", cfg.AIModel, "AI model name")

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
	cfg.AIProvider = strings.ToLower(strings.TrimSpace(cfg.AIProvider))
	cfg.AIEndpoint = strings.TrimSpace(cfg.AIEndpoint)
	cfg.AIModel = strings.TrimSpace(cfg.AIModel)
	cfg.CloudAIEndpoint = strings.TrimSpace(cfg.CloudAIEndpoint)
	cfg.CloudAIModel = strings.TrimSpace(cfg.CloudAIModel)
	cfg.CloudAIAPIKey = strings.TrimSpace(cfg.CloudAIAPIKey)
	switch cfg.AIProvider {
	case "":
		if cfg.AIModel != "" || cfg.AIEndpoint != "" {
			return Config{}, fmt.Errorf("AI provider is required when AI model/endpoint is configured")
		}
	case "ollama":
		if cfg.AIModel == "" {
			return Config{}, fmt.Errorf("AI model is required for ollama provider")
		}
	default:
		return Config{}, fmt.Errorf("unsupported AI provider %q", cfg.AIProvider)
	}

	if cfg.CloudAIEndpoint != "" || cfg.CloudAIModel != "" || cfg.CloudAIAPIKey != "" {
		if cfg.CloudAIEndpoint == "" || cfg.CloudAIModel == "" || cfg.CloudAIAPIKey == "" {
			return Config{}, fmt.Errorf("cloud AI endpoint, model and API key must be configured together")
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
