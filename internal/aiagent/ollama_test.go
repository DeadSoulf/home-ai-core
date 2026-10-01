package aiagent

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOllamaProviderModelsAndStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"models":[{"name":"home-model","model":"home-model:latest","size":1234,"details":{"family":"test","parameter_size":"3B","quantization_level":"Q4"}}]}`)
		case "/api/chat":
			w.Header().Set("Content-Type", "application/x-ndjson")
			_, _ = fmt.Fprintln(w, `{"message":{"role":"assistant","content":"hello "},"done":false}`)
			_, _ = fmt.Fprintln(w, `{"message":{"role":"assistant","content":"home"},"done":true}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider, err := NewOllamaProvider(server.URL)
	if err != nil {
		t.Fatalf("NewOllamaProvider() error = %v", err)
	}
	models, err := provider.Models(context.Background())
	if err != nil {
		t.Fatalf("Models() error = %v", err)
	}
	if len(models) != 1 || models[0].ID != "home-model:latest" || models[0].Family != "test" {
		t.Fatalf("models = %#v", models)
	}

	var streamed strings.Builder
	response, err := provider.Stream(
		context.Background(),
		ModelRequest{
			Model:    "home-model:latest",
			Messages: []Message{{Role: RoleUser, Content: "hello"}},
		},
		func(delta string) error {
			streamed.WriteString(delta)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if streamed.String() != "hello home" || response.Message.Content != "hello home" {
		t.Fatalf("streamed=%q response=%#v", streamed.String(), response)
	}
}

func TestOllamaProviderRejectsInvalidURL(t *testing.T) {
	for _, value := range []string{"ftp://localhost:11434", "http://user:pass@localhost:11434", "://bad"} {
		if _, err := NewOllamaProvider(value); err == nil {
			t.Fatalf("NewOllamaProvider(%q) accepted invalid URL", value)
		}
	}
}
