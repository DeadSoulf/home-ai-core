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

const maxOpenAICompatibleResponseBytes = 4 << 20

type OpenAICompatibleProvider struct {
	endpoint string
	model    string
	apiKey   string
	client   *http.Client
}

type openAICompatibleFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

type openAICompatibleTool struct {
	Type     string                   `json:"type"`
	Function openAICompatibleFunction `json:"function"`
}

type openAICompatibleToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAICompatibleToolCall struct {
	ID       string                           `json:"id"`
	Type     string                           `json:"type,omitempty"`
	Function openAICompatibleToolCallFunction `json:"function"`
}

type openAICompatibleMessage struct {
	Role       string                     `json:"role"`
	Content    string                     `json:"content,omitempty"`
	ToolCalls  []openAICompatibleToolCall `json:"tool_calls,omitempty"`
	ToolCallID string                     `json:"tool_call_id,omitempty"`
}

func NewOpenAICompatibleProvider(endpoint, model, apiKey string) (*OpenAICompatibleProvider, error) {
	endpoint = strings.TrimSpace(endpoint)
	model = strings.TrimSpace(model)
	apiKey = strings.TrimSpace(apiKey)
	if endpoint == "" {
		return nil, errors.New("cloud AI endpoint is required")
	}
	if model == "" {
		return nil, errors.New("cloud AI model is required")
	}
	if apiKey == "" {
		return nil, errors.New("cloud AI API key is required")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, errors.New("cloud AI endpoint must be an absolute http/https URL")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("cloud AI endpoint must not contain query or fragment")
	}
	path := strings.TrimRight(parsed.Path, "/")
	if !strings.HasSuffix(path, "/chat/completions") {
		path += "/chat/completions"
	}
	parsed.Path = path
	return &OpenAICompatibleProvider{
		endpoint: parsed.String(),
		model:    model,
		apiKey:   apiKey,
		client:   &http.Client{Timeout: 95 * time.Second},
	}, nil
}

func (p *OpenAICompatibleProvider) ID() string {
	return "cloud.openai-compatible"
}

func (p *OpenAICompatibleProvider) Model() string {
	return p.model
}


func cloudProviderErrorKind(status int) string {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return "authentication"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusTooManyRequests:
		return "rate_limit"
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return "invalid_request"
	default:
		if status >= 500 {
			return "unavailable"
		}
		return "http_error"
	}
}

func cloudProviderErrorMessage(status int, detail string) string {
	var message string
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		message = "Cloud AI authentication failed. Check the API key"
	case http.StatusNotFound:
		message = "Cloud AI endpoint or model was not found. Check endpoint and model"
	case http.StatusTooManyRequests:
		message = "Cloud AI rate limit or quota was reached"
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		message = "Cloud AI rejected the request. Check model and provider compatibility"
	default:
		if status >= 500 {
			message = "Cloud AI provider is temporarily unavailable"
		} else {
			message = fmt.Sprintf("Cloud AI returned HTTP %d", status)
		}
	}
	if detail != "" {
		message += ": " + detail
	}
	return message
}

func cloudProviderErrorDetail(raw []byte, apiKey string) string {
	var decoded struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    any    `json:"code"`
		} `json:"error"`
	}
	detail := ""
	if json.Unmarshal(raw, &decoded) == nil {
		detail = strings.TrimSpace(decoded.Error.Message)
	}
	if detail == "" {
		return ""
	}
	detail = strings.ReplaceAll(detail, "\r", " ")
	detail = strings.ReplaceAll(detail, "\n", " ")
	detail = strings.Join(strings.Fields(detail), " ")
	if apiKey != "" {
		detail = strings.ReplaceAll(detail, apiKey, "[redacted]")
	}
	if len(detail) > 300 {
		detail = detail[:300] + "…"
	}
	return detail
}

func (p *OpenAICompatibleProvider) Generate(ctx context.Context, request ModelRequest) (ModelResponse, error) {
	if p == nil || p.client == nil {
		return ModelResponse{}, ErrProviderUnavailable
	}

	toolNameByID := make(map[string]string, len(request.Tools))
	toolIDByName := make(map[string]string, len(request.Tools))
	tools := make([]openAICompatibleTool, 0, len(request.Tools))
	for _, descriptor := range request.Tools {
		name := ollamaToolName(descriptor.ID)
		if existing, ok := toolIDByName[name]; ok && existing != descriptor.ID {
			return ModelResponse{}, fmt.Errorf("cloud AI tool-name collision between %q and %q", existing, descriptor.ID)
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
		tools = append(tools, openAICompatibleTool{
			Type: "function",
			Function: openAICompatibleFunction{
				Name:        name,
				Description: descriptor.Description,
				Parameters:  parameters,
			},
		})
	}

	messages := make([]openAICompatibleMessage, 0, len(request.Messages))
	for _, message := range request.Messages {
		switch message.Role {
		case RoleSystem, RoleUser, RoleAssistant, RoleTool:
		default:
			return ModelResponse{}, fmt.Errorf("unsupported AI message role %q", message.Role)
		}
		out := openAICompatibleMessage{
			Role:       string(message.Role),
			Content:    message.Content,
			ToolCallID: message.ToolCallID,
		}
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
			out.ToolCalls = append(out.ToolCalls, openAICompatibleToolCall{
				ID:   call.ID,
				Type: "function",
				Function: openAICompatibleToolCallFunction{
					Name:      name,
					Arguments: string(input),
				},
			})
		}
		messages = append(messages, out)
	}

	payload := map[string]any{
		"model":    p.model,
		"messages": messages,
		"stream":   false,
	}
	if len(tools) > 0 {
		payload["tools"] = tools
		payload["tool_choice"] = "auto"
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return ModelResponse{}, fmt.Errorf("encode cloud AI request: %w", err)
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return ModelResponse{}, fmt.Errorf("create cloud AI request: %w", err)
	}
	httpRequest.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")
	httpRequest.Header.Set("User-Agent", "Home-AI-Core")

	response, err := p.client.Do(httpRequest)
	if err != nil {
		return ModelResponse{}, &ProviderRequestError{
			Provider: "cloud",
			Kind:     "network",
			Message:  "Cloud AI connection failed",
			Err:      err,
		}
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxOpenAICompatibleResponseBytes+1))
	if err != nil {
		return ModelResponse{}, fmt.Errorf("read cloud AI response: %w", err)
	}
	if len(raw) > maxOpenAICompatibleResponseBytes {
		return ModelResponse{}, errors.New("cloud AI response exceeds size limit")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		detail := cloudProviderErrorDetail(raw, p.apiKey)
		return ModelResponse{}, &ProviderRequestError{
			Provider:   "cloud",
			Kind:       cloudProviderErrorKind(response.StatusCode),
			StatusCode: response.StatusCode,
			Message:    cloudProviderErrorMessage(response.StatusCode, detail),
		}
	}

	var decoded struct {
		Choices []struct {
			Message openAICompatibleMessage `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return ModelResponse{}, fmt.Errorf("decode cloud AI response: %w", err)
	}
	if len(decoded.Choices) == 0 {
		return ModelResponse{}, errors.New("cloud AI returned no choices")
	}
	message := decoded.Choices[0].Message
	calls := make([]ToolCall, 0, len(message.ToolCalls))
	for index, call := range message.ToolCalls {
		toolID, ok := toolIDByName[call.Function.Name]
		if !ok {
			return ModelResponse{}, fmt.Errorf("cloud AI returned unknown tool %q", call.Function.Name)
		}
		input := json.RawMessage(call.Function.Arguments)
		if len(input) == 0 || string(input) == "null" {
			input = json.RawMessage(`{}`)
		}
		if !json.Valid(input) {
			return ModelResponse{}, fmt.Errorf("cloud AI returned invalid arguments for tool %q", toolID)
		}
		callID := strings.TrimSpace(call.ID)
		if callID == "" {
			callID = fmt.Sprintf("cloud_%d", index+1)
		}
		calls = append(calls, ToolCall{
			ID:     callID,
			ToolID: toolID,
			Input:  append(json.RawMessage(nil), input...),
		})
	}

	content := strings.TrimSpace(message.Content)
	if content == "" && len(calls) == 0 {
		return ModelResponse{}, errors.New("cloud AI returned an empty response")
	}
	out := Message{Role: RoleAssistant, Content: content, ToolCalls: calls}
	return ModelResponse{Message: out, ToolCalls: calls}, nil
}
