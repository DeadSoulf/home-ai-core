package aiagent

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/state"
)

func TestPersistentConversationUsesLocalProviderAndUserScope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"models":[{"name":"home-model","model":"home-model","size":123}]}`)
		case "/api/chat":
			w.Header().Set("Content-Type", "application/x-ndjson")
			_, _ = fmt.Fprintln(w, `{"message":{"role":"assistant","content":"Hello "},"done":false}`)
			_, _ = fmt.Fprintln(w, `{"message":{"role":"assistant","content":"from Home-AI"},"done":true}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	store, err := state.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	now := time.Now().UTC()
	if _, err := store.CreateOwner(context.Background(), "usr-owner", "owner", "Owner", "hash", now); err != nil {
		t.Fatal(err)
	}

	provider, err := NewOllamaProvider(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	audit := &serviceAudit{}
	service := NewService("node-1", store, nil, nil, audit)
	service.SetProvider(provider)

	actor := security.Actor{Type: "user", ID: "usr-owner"}
	conversation, err := service.CreateConversation(context.Background(), actor, "home-model")
	if err != nil {
		t.Fatalf("CreateConversation() error = %v", err)
	}
	if conversation.Model != "home-model" || conversation.Provider != "ollama" {
		t.Fatalf("conversation = %#v", conversation)
	}

	var streamed strings.Builder
	result, err := service.StreamConversation(
		context.Background(),
		actor,
		security.RequestContext{RequestID: "req-chat"},
		conversation.ID,
		"How is home?",
		func(delta string) error {
			streamed.WriteString(delta)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("StreamConversation() error = %v", err)
	}
	if streamed.String() != "Hello from Home-AI" {
		t.Fatalf("stream = %q", streamed.String())
	}
	if result.Title != "How is home?" || len(result.Messages) != 2 {
		t.Fatalf("result = %#v", result)
	}
	if result.Messages[0].Role != "user" || result.Messages[1].Role != "assistant" {
		t.Fatalf("messages = %#v", result.Messages)
	}
	if len(audit.calls) != 1 || audit.calls[0].action != "ai.chat.message" || audit.calls[0].outcome != "success" {
		t.Fatalf("audit = %#v", audit.calls)
	}
	if _, ok := audit.calls[0].metadata["content"]; ok {
		t.Fatalf("chat content leaked into audit: %#v", audit.calls[0].metadata)
	}

	other := security.Actor{Type: "user", ID: "usr-other"}
	if _, err := service.Conversation(context.Background(), other, conversation.ID); err == nil {
		t.Fatal("other user could read conversation")
	}
}
