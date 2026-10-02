package aiagent

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
)

var ErrProviderUnavailable = errors.New("AI provider is unavailable")

type ProviderRequestError struct {
	Provider   string
	Kind       string
	StatusCode int
	Message    string
	Err        error
}

func (e *ProviderRequestError) Error() string {
	if e == nil {
		return "AI provider request failed"
	}
	if e.Message != "" {
		return e.Message
	}
	if e.StatusCode > 0 {
		return e.Provider + " provider returned HTTP " + strconv.Itoa(e.StatusCode)
	}
	if e.Err != nil {
		return e.Provider + " provider request failed: " + e.Err.Error()
	}
	return e.Provider + " provider request failed"
}

func (e *ProviderRequestError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type MessageRole string

const (
	RoleSystem    MessageRole = "system"
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleTool      MessageRole = "tool"
)

type Message struct {
	Role       MessageRole `json:"role"`
	Content    string      `json:"content"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"`
}

type ToolCall struct {
	ID     string          `json:"id"`
	ToolID string          `json:"tool_id"`
	Input  json.RawMessage `json:"input,omitempty"`
}

type ModelRequest struct {
	Messages     []Message        `json:"messages"`
	Tools        []ToolDescriptor `json:"tools,omitempty"`
	ProviderMode string           `json:"provider_mode,omitempty"`
}

type ModelResponse struct {
	Message   Message    `json:"message"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

type Provider interface {
	ID() string
	Generate(context.Context, ModelRequest) (ModelResponse, error)
}

type StreamingProvider interface {
	Provider
	GenerateStream(context.Context, ModelRequest, func(string) error) (ModelResponse, error)
}

type DeterministicProvider struct {
	ProviderID string
	Response   ModelResponse
}

func (p DeterministicProvider) ID() string {
	if p.ProviderID == "" {
		return "deterministic"
	}
	return p.ProviderID
}

func (p DeterministicProvider) Generate(ctx context.Context, _ ModelRequest) (ModelResponse, error) {
	select {
	case <-ctx.Done():
		return ModelResponse{}, ctx.Err()
	default:
	}
	if p.Response.Message.Role == "" {
		return ModelResponse{}, errors.New("deterministic provider response is not configured")
	}
	return p.Response, nil
}
