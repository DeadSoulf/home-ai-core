package aiagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxOllamaResponseBytes = 4 << 20

type OllamaProvider struct {
	endpoint string
	model    string
	client   *http.Client
}

func NewOllamaProvider(endpoint, model string) (*OllamaProvider, error) {
	endpoint = strings.TrimSpace(endpoint)
	model = strings.TrimSpace(model)
	if endpoint == "" {
		endpoint = "http://127.0.0.1:11434"
	}
	if model == "" {
		return nil, errors.New("Ollama model is required")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, errors.New("Ollama endpoint must be an absolute http/https URL")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("Ollama endpoint must not contain query or fragment")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return &OllamaProvider{
		endpoint: parsed.String(),
		model:    model,
		client:   &http.Client{Timeout: 90 * time.Second},
	}, nil
}

func (p *OllamaProvider) ID() string {
	return "ollama"
}

func (p *OllamaProvider) Model() string {
	return p.model
}

func (p *OllamaProvider) Generate(ctx context.Context, request ModelRequest) (ModelResponse, error) {
	if p == nil || p.client == nil {
		return ModelResponse{}, errors.New("Ollama provider is not initialized")
	}

	type ollamaMessage struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	messages := make([]ollamaMessage, 0, len(request.Messages))
	for _, message := range request.Messages {
		switch message.Role {
		case RoleSystem, RoleUser, RoleAssistant, RoleTool:
		default:
			return ModelResponse{}, fmt.Errorf("unsupported AI message role %q", message.Role)
		}
		messages = append(messages, ollamaMessage{Role: string(message.Role), Content: message.Content})
	}

	body, err := json.Marshal(map[string]any{
		"model":    p.model,
		"messages": messages,
		"stream":   false,
	})
	if err != nil {
		return ModelResponse{}, fmt.Errorf("encode Ollama request: %w", err)
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return ModelResponse{}, fmt.Errorf("create Ollama request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")

	response, err := p.client.Do(httpRequest)
	if err != nil {
		return ModelResponse{}, fmt.Errorf("Ollama request failed: %w", err)
	}
	defer response.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(response.Body, maxOllamaResponseBytes+1))
	if err != nil {
		return ModelResponse{}, fmt.Errorf("read Ollama response: %w", err)
	}
	if len(raw) > maxOllamaResponseBytes {
		return ModelResponse{}, errors.New("Ollama response exceeds size limit")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return ModelResponse{}, fmt.Errorf("Ollama returned HTTP %d", response.StatusCode)
	}

	var decoded struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return ModelResponse{}, fmt.Errorf("decode Ollama response: %w", err)
	}
	if strings.TrimSpace(decoded.Message.Content) == "" {
		return ModelResponse{}, errors.New("Ollama returned an empty response")
	}
	return ModelResponse{
		Message: Message{Role: RoleAssistant, Content: decoded.Message.Content},
	}, nil
}
