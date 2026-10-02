package state

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAIConversationsAreUserScopedAndPersistent(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	now := time.Unix(100, 0).UTC()
	conv, err := store.CreateAIConversation(ctx, "aic-1", "usr-1", "First chat", now)
	if err != nil {
		t.Fatal(err)
	}
	if conv.UserID != "usr-1" || conv.Title != "First chat" {
		t.Fatalf("conversation = %#v", conv)
	}

	if _, err := store.AppendAIMessage(ctx, "aim-1", conv.ID, "usr-1", "user", "hello", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendAIMessage(ctx, "aim-2", conv.ID, "usr-1", "assistant", "hi", now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}

	messages, err := store.ListAIMessages(ctx, conv.ID, "usr-1", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].Content != "hello" || messages[1].Content != "hi" {
		t.Fatalf("messages = %#v", messages)
	}

	if _, err := store.AIConversation(ctx, conv.ID, "usr-2"); !errors.Is(err, ErrAIConversationNotFound) {
		t.Fatalf("foreign AIConversation() error = %v", err)
	}
	if _, err := store.ListAIMessages(ctx, conv.ID, "usr-2", 20); !errors.Is(err, ErrAIConversationNotFound) {
		t.Fatalf("foreign ListAIMessages() error = %v", err)
	}
	if _, err := store.AppendAIMessage(ctx, "aim-x", conv.ID, "usr-2", "user", "steal", now); !errors.Is(err, ErrAIConversationNotFound) {
		t.Fatalf("foreign AppendAIMessage() error = %v", err)
	}

	list, err := store.ListAIConversations(ctx, "usr-1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != conv.ID {
		t.Fatalf("conversations = %#v", list)
	}
}
