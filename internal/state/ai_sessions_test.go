package state

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAISessionsAreScopedToUser(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	now := time.Unix(100, 0).UTC()
	user, err := store.CreateOwner(ctx, "usr-owner", "owner", "Owner", "hash", now)
	if err != nil {
		t.Fatalf("CreateOwner() error = %v", err)
	}

	session := AISessionRecord{
		ID:        "ais-one",
		UserID:    user.ID,
		Provider:  "ollama",
		Model:     "test-model",
		Title:     "Test",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := store.CreateAISession(ctx, session); err != nil {
		t.Fatalf("CreateAISession() error = %v", err)
	}

	if _, err := store.AISession(ctx, session.ID, user.ID); err != nil {
		t.Fatalf("AISession() owner error = %v", err)
	}
	if _, err := store.AISession(ctx, session.ID, "usr-other"); !errors.Is(err, ErrAISessionNotFound) {
		t.Fatalf("AISession() other user error = %v, want not found", err)
	}

	message := AIMessageRecord{
		ID:        "aim-one",
		SessionID: session.ID,
		Role:      "user",
		Content:   "hello",
		CreatedAt: now.Add(time.Second),
	}
	if err := store.AppendAIMessage(ctx, user.ID, message); err != nil {
		t.Fatalf("AppendAIMessage() error = %v", err)
	}
	if err := store.AppendAIMessage(ctx, "usr-other", AIMessageRecord{
		ID:        "aim-denied",
		SessionID: session.ID,
		Role:      "user",
		Content:   "should fail",
		CreatedAt: now.Add(2 * time.Second),
	}); !errors.Is(err, ErrAISessionNotFound) {
		t.Fatalf("AppendAIMessage() other user error = %v, want not found", err)
	}

	messages, err := store.ListAIMessages(ctx, session.ID, user.ID, 100)
	if err != nil {
		t.Fatalf("ListAIMessages() error = %v", err)
	}
	if len(messages) != 1 || messages[0].Content != "hello" {
		t.Fatalf("messages = %#v", messages)
	}
	if _, err := store.ListAIMessages(ctx, session.ID, "usr-other", 100); !errors.Is(err, ErrAISessionNotFound) {
		t.Fatalf("ListAIMessages() other user error = %v, want not found", err)
	}
}

func TestAISessionListAndTitleUpdate(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	now := time.Unix(200, 0).UTC()
	user, err := store.CreateOwner(ctx, "usr-owner", "owner", "Owner", "hash", now)
	if err != nil {
		t.Fatalf("CreateOwner() error = %v", err)
	}
	if err := store.CreateAISession(ctx, AISessionRecord{
		ID: "ais-one", UserID: user.ID, Provider: "ollama", Model: "m1",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateAISessionTitle(ctx, "ais-one", user.ID, "First question", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	items, err := store.ListAISessions(ctx, user.ID, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Title != "First question" {
		t.Fatalf("sessions = %#v", items)
	}
}
