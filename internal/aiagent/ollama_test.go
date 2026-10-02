package aiagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOllamaProviderGenerate(t *testing.T) {
	var gotModel string
	var gotMessages []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		var body struct {
			Model     string           `json:"model"`
			Messages  []map[string]any `json:"messages"`
			Stream    bool             `json:"stream"`
			Think     bool             `json:"think"`
			KeepAlive string           `json:"keep_alive"`
			Options   struct {
				NumCtx int `json:"num_ctx"`
			} `json:"options"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		gotModel = body.Model
		gotMessages = body.Messages
		if body.Stream {
			t.Fatal("stream unexpectedly enabled")
		}
		if body.Think {
			t.Fatal("thinking should be disabled in the default fast mode")
		}
		if body.KeepAlive != defaultOllamaKeepAlive {
			t.Fatalf("keep_alive = %q, want %q", body.KeepAlive, defaultOllamaKeepAlive)
		}
		if body.Options.NumCtx != defaultOllamaContext {
			t.Fatalf("num_ctx = %d, want %d", body.Options.NumCtx, defaultOllamaContext)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"Local answer"}}`))
	}))
	defer server.Close()

	provider, err := NewOllamaProvider(server.URL, "test-model")
	if err != nil {
		t.Fatal(err)
	}
	response, err := provider.Generate(context.Background(), ModelRequest{
		Messages: []Message{{Role: RoleUser, Content: "hello"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotModel != "test-model" || len(gotMessages) != 1 {
		t.Fatalf("request model/messages = %q %#v", gotModel, gotMessages)
	}
	if response.Message.Role != RoleAssistant || response.Message.Content != "Local answer" {
		t.Fatalf("response = %#v", response)
	}
}

func TestOllamaProviderRejectsInvalidConfiguration(t *testing.T) {
	if _, err := NewOllamaProvider("file:///tmp/model", "model"); err == nil {
		t.Fatal("file endpoint was accepted")
	}
	if _, err := NewOllamaProvider("http://127.0.0.1:11434", ""); err == nil {
		t.Fatal("empty model was accepted")
	}
}

func TestOllamaProviderMapsTypedToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Tools []struct {
				Type     string `json:"type"`
				Function struct {
					Name       string          `json:"name"`
					Parameters json.RawMessage `json:"parameters"`
				} `json:"function"`
			} `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body.Tools) != 1 {
			t.Fatalf("tools = %#v", body.Tools)
		}
		if body.Tools[0].Type != "function" || body.Tools[0].Function.Name != "home_ai_core_system_status" {
			t.Fatalf("tool = %#v", body.Tools[0])
		}
		if !json.Valid(body.Tools[0].Function.Parameters) {
			t.Fatalf("invalid parameters = %s", body.Tools[0].Function.Parameters)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"message":{
				"role":"assistant",
				"content":"",
				"tool_calls":[{"function":{"name":"home_ai_core_system_status","arguments":{}}}]
			}
		}`))
	}))
	defer server.Close()

	provider, err := NewOllamaProvider(server.URL, "tool-model")
	if err != nil {
		t.Fatal(err)
	}
	response, err := provider.Generate(context.Background(), ModelRequest{
		Messages: []Message{{Role: RoleUser, Content: "inspect the server"}},
		Tools: []ToolDescriptor{{
			ID:          "core.system.status",
			ModuleID:    "ai.agent",
			Name:        "System status",
			Description: "Read system status",
			InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
			Sensitivity: SensitivityRead,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.ToolCalls) != 1 {
		t.Fatalf("tool calls = %#v", response.ToolCalls)
	}
	if response.ToolCalls[0].ToolID != "core.system.status" || string(response.ToolCalls[0].Input) != "{}" {
		t.Fatalf("tool call = %#v", response.ToolCalls[0])
	}
	if len(response.Message.ToolCalls) != 1 {
		t.Fatalf("message tool calls = %#v", response.Message.ToolCalls)
	}
}

func TestOllamaProviderGenerateStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Stream bool `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if !body.Stream {
			t.Fatal("streaming request did not enable stream")
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write([]byte("{\"message\":{\"role\":\"assistant\",\"content\":\"Fast \"},\"done\":false}\n"))
		_, _ = w.Write([]byte("{\"message\":{\"role\":\"assistant\",\"content\":\"reply\"},\"done\":true}\n"))
	}))
	defer server.Close()

	provider, err := NewOllamaProvider(server.URL, "test-model")
	if err != nil {
		t.Fatal(err)
	}
	var streamed strings.Builder
	response, err := provider.GenerateStream(
		context.Background(),
		ModelRequest{Messages: []Message{{Role: RoleUser, Content: "hello"}}},
		func(delta string) error {
			streamed.WriteString(delta)
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if streamed.String() != "Fast reply" {
		t.Fatalf("streamed = %q", streamed.String())
	}
	if response.Message.Content != "Fast reply" {
		t.Fatalf("response = %#v", response)
	}
}
