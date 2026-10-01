package aiagent

import (
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

const defaultOllamaURL = "http://127.0.0.1:11434"

type OllamaProvider struct {
	baseURL string
	client  *http.Client
}

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

type ollamaChatResponse struct {
	Message ollamaMessage `json:"message"`
	Done    bool          `json:"done"`
	Error   string        `json:"error,omitempty"`
}

type ollamaTagsResponse struct {
	Models []struct {
		Name  string `json:"name"`
		Model string `json:"model"`
		Size  int64  `json:"size"`
		Details struct {
			Family            string `json:"family"`
			ParameterSize     string `json:"parameter_size"`
			QuantizationLevel string `json:"quantization_level"`
		} `json:"details"`
	} `json:"models"`
}

func NewOllamaProvider(baseURL string) (*OllamaProvider, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = defaultOllamaURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil {
		return nil, errors.New("invalid Ollama base URL")
	}
	return &OllamaProvider{
		baseURL: strings.TrimRight(parsed.String(), "/"),
		client: &http.Client{Timeout: 0},
	}, nil
}

func (p *OllamaProvider) ID() string { return "ollama" }

func (p *OllamaProvider) Models(ctx context.Context) ([]ModelInfo, error) {
	requestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, p.baseURL+"/api/tags", nil)
	if err != nil { return nil, err }
	resp, err := p.client.Do(req)
	if err != nil { return nil, fmt.Errorf("query Ollama models: %w", err) }
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("query Ollama models: HTTP %d", resp.StatusCode)
	}
	var payload ollamaTagsResponse
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 4<<20))
	if err := decoder.Decode(&payload); err != nil { return nil, fmt.Errorf("decode Ollama models: %w", err) }
	models := make([]ModelInfo, 0, len(payload.Models))
	for _, item := range payload.Models {
		id := strings.TrimSpace(item.Model)
		if id == "" { id = strings.TrimSpace(item.Name) }
		if id == "" { continue }
		details := strings.TrimSpace(strings.Join([]string{item.Details.ParameterSize, item.Details.QuantizationLevel}, " "))
		models = append(models, ModelInfo{ID: id, Name: item.Name, Size: item.Size, Family: item.Details.Family, Details: details})
	}
	return models, nil
}

func (p *OllamaProvider) Generate(ctx context.Context, request ModelRequest) (ModelResponse, error) {
	if strings.TrimSpace(request.Model) == "" { return ModelResponse{}, errors.New("model is required") }
	body, err := json.Marshal(ollamaChatRequest{Model: request.Model, Messages: toOllamaMessages(request.Messages), Stream: false})
	if err != nil { return ModelResponse{}, err }
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/api/chat", strings.NewReader(string(body)))
	if err != nil { return ModelResponse{}, err }
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil { return ModelResponse{}, fmt.Errorf("Ollama chat: %w", err) }
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ModelResponse{}, fmt.Errorf("Ollama chat: HTTP %d", resp.StatusCode)
	}
	var payload ollamaChatResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&payload); err != nil { return ModelResponse{}, fmt.Errorf("decode Ollama chat: %w", err) }
	if payload.Error != "" { return ModelResponse{}, errors.New(payload.Error) }
	return ModelResponse{Message: Message{Role: RoleAssistant, Content: payload.Message.Content}}, nil
}

func (p *OllamaProvider) Stream(ctx context.Context, request ModelRequest, onDelta func(string) error) (ModelResponse, error) {
	if strings.TrimSpace(request.Model) == "" { return ModelResponse{}, errors.New("model is required") }
	body, err := json.Marshal(ollamaChatRequest{Model: request.Model, Messages: toOllamaMessages(request.Messages), Stream: true})
	if err != nil { return ModelResponse{}, err }
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/api/chat", strings.NewReader(string(body)))
	if err != nil { return ModelResponse{}, err }
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil { return ModelResponse{}, fmt.Errorf("Ollama chat stream: %w", err) }
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK { return ModelResponse{}, fmt.Errorf("Ollama chat stream: HTTP %d", resp.StatusCode) }

	decoder := json.NewDecoder(io.LimitReader(resp.Body, 64<<20))
	var full strings.Builder
	for {
		var chunk ollamaChatResponse
		err := decoder.Decode(&chunk)
		if errors.Is(err, io.EOF) { break }
		if err != nil { return ModelResponse{}, fmt.Errorf("decode Ollama stream: %w", err) }
		if chunk.Error != "" { return ModelResponse{}, errors.New(chunk.Error) }
		if chunk.Message.Content != "" {
			full.WriteString(chunk.Message.Content)
			if onDelta != nil {
				if err := onDelta(chunk.Message.Content); err != nil { return ModelResponse{}, err }
			}
		}
		if chunk.Done { break }
	}
	return ModelResponse{Message: Message{Role: RoleAssistant, Content: full.String()}}, nil
}

func toOllamaMessages(messages []Message) []ollamaMessage {
	result := make([]ollamaMessage, 0, len(messages))
	for _, message := range messages {
		result = append(result, ollamaMessage{Role: string(message.Role), Content: message.Content})
	}
	return result
}
