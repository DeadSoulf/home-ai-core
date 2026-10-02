package aiagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
			Model    string           `json:"model"`
			Messages []map[string]any `json:"messages"`
			Stream   bool             `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		gotModel = body.Model
		gotMessages = body.Messages
		if body.Stream {
			t.Fatal("stream unexpectedly enabled")
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
