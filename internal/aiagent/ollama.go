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
	"unicode"
)

const (
	maxOllamaResponseBytes = 4 << 20
	defaultOllamaKeepAlive = "30m"
	defaultOllamaContext   = 16384
)

type OllamaProvider struct {
	endpoint    string
	model       string
	keepAlive   string
	contextSize int
	think       bool
	client      *http.Client
}

type ollamaFunctionDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

type ollamaToolDefinition struct {
	Type     string                   `json:"type"`
	Function ollamaFunctionDefinition `json:"function"`
}

type ollamaFunctionCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type ollamaToolCall struct {
	Function ollamaFunctionCall `json:"function"`
}

type ollamaMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
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
		endpoint:    parsed.String(),
		model:       model,
		keepAlive:   defaultOllamaKeepAlive,
		contextSize: defaultOllamaContext,
		think:       false,
		client:      &http.Client{Timeout: 95 * time.Second},
	}, nil
}

func (p *OllamaProvider) ID() string {
	return "ollama"
}

func (p *OllamaProvider) Model() string {
	return p.model
}

func (p *OllamaProvider) Generate(ctx context.Context, request ModelRequest) (ModelResponse, error) {
	return p.generate(ctx, request, false, nil)
}

func (p *OllamaProvider) GenerateStream(
	ctx context.Context,
	request ModelRequest,
	onContent func(string) error,
) (ModelResponse, error) {
	return p.generate(ctx, request, true, onContent)
}

func (p *OllamaProvider) generate(
	ctx context.Context,
	request ModelRequest,
	stream bool,
	onContent func(string) error,
) (ModelResponse, error) {
	if p == nil || p.client == nil {
		return ModelResponse{}, errors.New("Ollama provider is not initialized")
	}

	toolNameByID := make(map[string]string, len(request.Tools))
	toolIDByName := make(map[string]string, len(request.Tools))
	tools := make([]ollamaToolDefinition, 0, len(request.Tools))
	for _, descriptor := range request.Tools {
		name := ollamaToolName(descriptor.ID)
		if existing, exists := toolIDByName[name]; exists && existing != descriptor.ID {
			return ModelResponse{}, fmt.Errorf("Ollama tool-name collision between %q and %q", existing, descriptor.ID)
		}
		parameters := descriptor.InputSchema
		if len(parameters) == 0 {
			parameters = json.RawMessage(`{"type":"object","additionalProperties":false}`)
		}
		if !json.Valid(parameters) {
			return ModelResponse{}, fmt.Errorf("invalid input schema for AI tool %q", descriptor.ID)
		}
		toolNameByID[descriptor.ID] = name
		toolIDByName[name] = descriptor.ID
		tools = append(tools, ollamaToolDefinition{
			Type: "function",
			Function: ollamaFunctionDefinition{
				Name:        name,
				Description: descriptor.Description,
				Parameters:  parameters,
			},
		})
	}

	messages := make([]ollamaMessage, 0, len(request.Messages))
	for _, message := range request.Messages {
		switch message.Role {
		case RoleSystem, RoleUser, RoleAssistant, RoleTool:
		default:
			return ModelResponse{}, fmt.Errorf("unsupported AI message role %q", message.Role)
		}
		out := ollamaMessage{Role: string(message.Role), Content: message.Content}
		for _, call := range message.ToolCalls {
			name, ok := toolNameByID[call.ToolID]
			if !ok {
				return ModelResponse{}, fmt.Errorf("AI message references unavailable tool %q", call.ToolID)
			}
			input := call.Input
			if len(input) == 0 {
				input = json.RawMessage(`{}`)
			}
			if !json.Valid(input) {
				return ModelResponse{}, fmt.Errorf("AI message tool input for %q is invalid JSON", call.ToolID)
			}
			out.ToolCalls = append(out.ToolCalls, ollamaToolCall{
				Function: ollamaFunctionCall{Name: name, Arguments: input},
			})
		}
		messages = append(messages, out)
	}

	payload := map[string]any{
		"model":      p.model,
		"messages":   messages,
		"stream":     stream,
		"think":      p.think,
		"keep_alive": p.keepAlive,
		"options": map[string]any{
			"num_ctx": p.contextSize,
		},
	}
	if len(tools) > 0 {
		payload["tools"] = tools
	}
	body, err := json.Marshal(payload)
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
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxOllamaResponseBytes))
		return ModelResponse{}, fmt.Errorf("Ollama returned HTTP %d", response.StatusCode)
	}

	var decodedMessage ollamaMessage
	if stream {
		limited := io.LimitReader(response.Body, maxOllamaResponseBytes+1)
		decoder := json.NewDecoder(limited)
		var content strings.Builder
		var toolCalls []ollamaToolCall
		for {
			var chunk struct {
				Message ollamaMessage `json:"message"`
				Done    bool          `json:"done"`
			}
			if err := decoder.Decode(&chunk); err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				return ModelResponse{}, fmt.Errorf("decode Ollama stream: %w", err)
			}
			if chunk.Message.Content != "" {
				content.WriteString(chunk.Message.Content)
				if onContent != nil {
					if err := onContent(chunk.Message.Content); err != nil {
						return ModelResponse{}, err
					}
				}
			}
			if len(chunk.Message.ToolCalls) > 0 {
				toolCalls = append(toolCalls, chunk.Message.ToolCalls...)
			}
			if chunk.Done {
				break
			}
		}
		decodedMessage = ollamaMessage{
			Role:      string(RoleAssistant),
			Content:   content.String(),
			ToolCalls: toolCalls,
		}
	} else {
		raw, err := io.ReadAll(io.LimitReader(response.Body, maxOllamaResponseBytes+1))
		if err != nil {
			return ModelResponse{}, fmt.Errorf("read Ollama response: %w", err)
		}
		if len(raw) > maxOllamaResponseBytes {
			return ModelResponse{}, errors.New("Ollama response exceeds size limit")
		}
		var decoded struct {
			Message ollamaMessage `json:"message"`
		}
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return ModelResponse{}, fmt.Errorf("decode Ollama response: %w", err)
		}
		decodedMessage = decoded.Message
	}

	calls := make([]ToolCall, 0, len(decodedMessage.ToolCalls))
	for index, call := range decodedMessage.ToolCalls {
		toolID, ok := toolIDByName[call.Function.Name]
		if !ok {
			return ModelResponse{}, fmt.Errorf("Ollama returned unknown tool %q", call.Function.Name)
		}
		input := call.Function.Arguments
		if len(input) == 0 || string(input) == "null" {
			input = json.RawMessage(`{}`)
		}
		if !json.Valid(input) {
			return ModelResponse{}, fmt.Errorf("Ollama returned invalid arguments for tool %q", toolID)
		}
		calls = append(calls, ToolCall{
			ID:     fmt.Sprintf("ollama_%d", index+1),
			ToolID: toolID,
			Input:  append(json.RawMessage(nil), input...),
		})
	}

	content := strings.TrimSpace(decodedMessage.Content)
	if content == "" && len(calls) == 0 {
		return ModelResponse{}, errors.New("Ollama returned an empty response")
	}
	message := Message{Role: RoleAssistant, Content: content, ToolCalls: calls}
	return ModelResponse{Message: message, ToolCalls: calls}, nil
}

func ollamaToolName(id string) string {
	var b strings.Builder
	b.WriteString("home_ai_")
	lastUnderscore := false
	for _, r := range id {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.TrimRight(b.String(), "_")
}
