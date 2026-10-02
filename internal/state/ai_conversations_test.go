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
	if _, err := store.CreateOwner(ctx, "usr-1", "owner", "Owner", "test-password-hash", now); err != nil {
		t.Fatal(err)
	}
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

	closed, err := store.CloseAIConversation(ctx, conv.ID, "usr-1", now.Add(3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if closed.ClosedAt == nil {
		t.Fatal("closed_at was not persisted")
	}
	if _, err := store.AppendAIMessage(ctx, "aim-3", conv.ID, "usr-1", "user", "after close", now.Add(4*time.Second)); !errors.Is(err, ErrAIConversationClosed) {
		t.Fatalf("AppendAIMessage() after close error = %v, want closed", err)
	}
	if _, err := store.CloseAIConversation(ctx, conv.ID, "usr-2", now); !errors.Is(err, ErrAIConversationNotFound) {
		t.Fatalf("foreign CloseAIConversation() error = %v", err)
	}
	messages, err = store.ListAIMessages(ctx, conv.ID, "usr-1", 20)
	if err != nil || len(messages) != 2 {
		t.Fatalf("history after close = %#v err=%v", messages, err)
	}

	list, err := store.ListAIConversations(ctx, "usr-1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != conv.ID {
		t.Fatalf("conversations = %#v", list)
	}
}


func TestDeleteClosedAIConversationsIsScopedAndRequiresClosedState(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	now := time.Unix(200, 0).UTC()
	if _, err := store.CreateOwner(ctx, "usr-clean", "cleaner", "Cleaner", "test-password-hash", now); err != nil {
		t.Fatal(err)
	}
	active, err := store.CreateAIConversation(ctx, "aic-active", "usr-clean", "Active", now)
	if err != nil {
		t.Fatal(err)
	}
	closedOne, err := store.CreateAIConversation(ctx, "aic-closed-1", "usr-clean", "Closed 1", now)
	if err != nil {
		t.Fatal(err)
	}
	closedTwo, err := store.CreateAIConversation(ctx, "aic-closed-2", "usr-clean", "Closed 2", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendAIMessage(ctx, "aim-clean-1", closedOne.ID, "usr-clean", "user", "history", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateAIToolAction(ctx, "aia-clean-1", closedOne.ID, "usr-clean", "core.system.status", "Status", "change", nil, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CloseAIConversation(ctx, closedOne.ID, "usr-clean", now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CloseAIConversation(ctx, closedTwo.ID, "usr-clean", now.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}

	if err := store.DeleteClosedAIConversation(ctx, active.ID, "usr-clean"); !errors.Is(err, ErrAIConversationNotClosed) {
		t.Fatalf("delete active conversation error = %v, want not closed", err)
	}
	if err := store.DeleteClosedAIConversation(ctx, closedOne.ID, "usr-other"); !errors.Is(err, ErrAIConversationNotFound) {
		t.Fatalf("foreign delete error = %v, want not found", err)
	}
	if err := store.DeleteClosedAIConversation(ctx, closedOne.ID, "usr-clean"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AIConversation(ctx, closedOne.ID, "usr-clean"); !errors.Is(err, ErrAIConversationNotFound) {
		t.Fatalf("deleted conversation lookup error = %v", err)
	}
	if _, err := store.ListAIMessages(ctx, closedOne.ID, "usr-clean", 10); !errors.Is(err, ErrAIConversationNotFound) {
		t.Fatalf("deleted message history lookup error = %v", err)
	}
	if _, err := store.AIToolAction(ctx, "aia-clean-1", closedOne.ID, "usr-clean"); !errors.Is(err, ErrAIToolActionNotFound) {
		t.Fatalf("deleted action lookup error = %v", err)
	}

	deleted, err := store.DeleteClosedAIConversations(ctx, "usr-clean")
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Fatalf("bulk deleted = %d, want 1", deleted)
	}
	if _, err := store.AIConversation(ctx, active.ID, "usr-clean"); err != nil {
		t.Fatalf("active conversation was removed: %v", err)
	}
	list, err := store.ListAIConversations(ctx, "usr-clean", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != active.ID {
		t.Fatalf("remaining conversations = %#v", list)
	}
}
