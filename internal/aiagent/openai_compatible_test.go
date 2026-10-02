package aiagent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAICompatibleProviderMapsToolsAndKeepsAPIKeyOutOfPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-secret" {
			t.Fatalf("authorization = %q", got)
		}
		var body struct {
			Model    string `json:"model"`
			Messages []struct {
				Role string `json:"role"`
			} `json:"messages"`
			Tools []struct {
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			} `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Model != "cloud-model" {
			t.Fatalf("model = %q", body.Model)
		}
		if len(body.Tools) != 1 || body.Tools[0].Function.Name != "home_ai_core_system_status" {
			t.Fatalf("tools = %#v", body.Tools)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"","tool_calls":[{"id":"call-1","type":"function","function":{"name":"home_ai_core_system_status","arguments":"{}"}}]}}]}`))
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(server.URL+"/v1", "cloud-model", "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	response, err := provider.Generate(context.Background(), ModelRequest{
		Messages: []Message{{Role: RoleUser, Content: "status"}},
		Tools: []ToolDescriptor{{
			ID:          "core.system.status",
			Description: "read status",
			InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.ToolCalls) != 1 {
		t.Fatalf("tool calls = %#v", response.ToolCalls)
	}
	if response.ToolCalls[0].ID != "call-1" || response.ToolCalls[0].ToolID != "core.system.status" {
		t.Fatalf("tool call = %#v", response.ToolCalls[0])
	}
}

func TestOpenAICompatibleProviderSendsToolCallID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []openAICompatibleMessage `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		last := body.Messages[len(body.Messages)-1]
		if last.Role != "tool" || last.ToolCallID != "call-9" {
			t.Fatalf("tool message = %#v", last)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"done"}}]}`))
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(server.URL, "cloud-model", "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.Generate(context.Background(), ModelRequest{
		Messages: []Message{
			{Role: RoleUser, Content: "status"},
			{Role: RoleTool, ToolCallID: "call-9", Content: `{"ok":true}`},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
}


func TestOpenAICompatibleProviderReportsAuthenticationFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad key test-secret"}}`))
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(server.URL, "cloud-model", "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.Generate(context.Background(), ModelRequest{
		Messages: []Message{{Role: RoleUser, Content: "test"}},
	})
	var providerErr *ProviderRequestError
	if !errors.As(err, &providerErr) {
		t.Fatalf("error = %T %v, want ProviderRequestError", err, err)
	}
	if providerErr.Kind != "authentication" || providerErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("provider error = %#v", providerErr)
	}
	if strings.Contains(providerErr.Error(), "test-secret") {
		t.Fatalf("provider error leaked API key: %q", providerErr.Error())
	}
	if !strings.Contains(providerErr.Error(), "[redacted]") {
		t.Fatalf("provider error did not redact provider detail: %q", providerErr.Error())
	}
}

func TestOpenAICompatibleProviderReportsIncompatibleSuccessPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"not-chat-completions"}`))
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(server.URL, "cloud-model", "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.Generate(context.Background(), ModelRequest{
		Messages: []Message{{Role: RoleUser, Content: "test"}},
	})
	var providerErr *ProviderRequestError
	if !errors.As(err, &providerErr) || providerErr.Kind != "invalid_response" {
		t.Fatalf("error = %T %v, want invalid_response ProviderRequestError", err, err)
	}
}
