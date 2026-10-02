package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("HOME_AI_LISTEN", "")
	t.Setenv("HOME_AI_STATE_DIR", "")
	t.Setenv("HOME_AI_WEB_DIR", "")
	t.Setenv("HOME_AI_AI_PROVIDER", "")
	t.Setenv("HOME_AI_AI_ENDPOINT", "")
	t.Setenv("HOME_AI_AI_MODEL", "")
	t.Setenv("HOME_AI_CLOUD_AI_ENDPOINT", "")
	t.Setenv("HOME_AI_CLOUD_AI_MODEL", "")
	t.Setenv("HOME_AI_CLOUD_AI_API_KEY", "")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.ListenAddress != defaultListen {
		t.Fatalf("ListenAddress = %q, want %q", cfg.ListenAddress, defaultListen)
	}
	if cfg.StateDir != defaultStateDir {
		t.Fatalf("StateDir = %q, want %q", cfg.StateDir, defaultStateDir)
	}
	if cfg.WebDir != defaultWebDir {
		t.Fatalf("WebDir = %q, want %q", cfg.WebDir, defaultWebDir)
	}
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv("HOME_AI_LISTEN", "127.0.0.1:9000")
	t.Setenv("HOME_AI_STATE_DIR", "/tmp/from-env")
	t.Setenv("HOME_AI_WEB_DIR", "/tmp/web-env")
	t.Setenv("HOME_AI_AI_PROVIDER", "ollama")
	t.Setenv("HOME_AI_AI_ENDPOINT", "http://127.0.0.1:11434")
	t.Setenv("HOME_AI_AI_MODEL", "qwen3:8b")
	t.Setenv("HOME_AI_CLOUD_AI_ENDPOINT", "https://api.example.com/v1")
	t.Setenv("HOME_AI_CLOUD_AI_MODEL", "cloud-model")
	t.Setenv("HOME_AI_CLOUD_AI_API_KEY", "secret-test-key")

	cfg, err := Load([]string{
		"-listen", "127.0.0.1:9100",
		"-state-dir", "/tmp/from-flag",
		"-web-dir", "/tmp/web-flag",
		"-ai-model", "qwen3:14b",
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.ListenAddress != "127.0.0.1:9100" {
		t.Fatalf("ListenAddress = %q", cfg.ListenAddress)
	}
	if cfg.StateDir != "/tmp/from-flag" {
		t.Fatalf("StateDir = %q", cfg.StateDir)
	}
	if cfg.WebDir != "/tmp/web-flag" {
		t.Fatalf("WebDir = %q", cfg.WebDir)
	}
	if cfg.AIProvider != "ollama" || cfg.AIEndpoint != "http://127.0.0.1:11434" || cfg.AIModel != "qwen3:14b" {
		t.Fatalf("AI config = %#v", cfg)
	}
	if cfg.CloudAIEndpoint != "https://api.example.com/v1" || cfg.CloudAIModel != "cloud-model" || cfg.CloudAIAPIKey != "secret-test-key" {
		t.Fatalf("Cloud AI config = %#v", cfg)
	}
}

func TestLoadRejectsIncompleteAIConfig(t *testing.T) {
	t.Setenv("HOME_AI_AI_PROVIDER", "ollama")
	t.Setenv("HOME_AI_AI_MODEL", "")
	if _, err := Load(nil); err == nil {
		t.Fatal("missing Ollama model was accepted")
	}
}


func TestLoadRejectsIncompleteCloudAIConfig(t *testing.T) {
	t.Setenv("HOME_AI_AI_PROVIDER", "")
	t.Setenv("HOME_AI_AI_ENDPOINT", "")
	t.Setenv("HOME_AI_AI_MODEL", "")
	t.Setenv("HOME_AI_CLOUD_AI_ENDPOINT", "https://api.example.com/v1")
	t.Setenv("HOME_AI_CLOUD_AI_MODEL", "cloud-model")
	t.Setenv("HOME_AI_CLOUD_AI_API_KEY", "")
	if _, err := Load(nil); err == nil {
		t.Fatal("incomplete Cloud AI configuration was accepted")
	}
}
