package state

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestAIToolActionsAreUserScopedAndSingleUse(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	now := time.Unix(200, 0).UTC()
	if _, err := store.CreateOwner(ctx, "usr-action", "action-owner", "Action Owner", "test-password-hash", now); err != nil {
		t.Fatal(err)
	}
	conversation, err := store.CreateAIConversation(ctx, "aic-action", "usr-action", "Server setup", now)
	if err != nil {
		t.Fatal(err)
	}

	action, err := store.CreateAIToolAction(
		ctx,
		"aia-1",
		conversation.ID,
		"usr-action",
		"core.storage.mount",
		"Mount storage",
		"change",
		json.RawMessage(`{"device":"/dev/sdb1"}`),
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if action.Status != "pending" || action.ToolID != "core.storage.mount" {
		t.Fatalf("action = %#v", action)
	}

	list, err := store.ListAIToolActions(ctx, conversation.ID, "usr-action", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != action.ID {
		t.Fatalf("actions = %#v", list)
	}
	if _, err := store.ListAIToolActions(ctx, conversation.ID, "usr-other", 20); !errors.Is(err, ErrAIConversationNotFound) {
		t.Fatalf("foreign ListAIToolActions() error = %v", err)
	}

	claimed, err := store.ClaimAIToolAction(ctx, action.ID, conversation.ID, "usr-action", now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if claimed.Status != "executing" {
		t.Fatalf("claimed status = %q", claimed.Status)
	}
	if _, err := store.ClaimAIToolAction(ctx, action.ID, conversation.ID, "usr-action", now.Add(2*time.Second)); !errors.Is(err, ErrAIToolActionNotPending) {
		t.Fatalf("second claim error = %v", err)
	}

	finished, err := store.FinishAIToolAction(
		ctx,
		action.ID,
		conversation.ID,
		"usr-action",
		"executed",
		json.RawMessage(`{"message":"mounted"}`),
		"",
		now.Add(3*time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	if finished.Status != "executed" || string(finished.Result) != `{"message":"mounted"}` {
		t.Fatalf("finished = %#v", finished)
	}

	rejectable, err := store.CreateAIToolAction(
		ctx,
		"aia-2",
		conversation.ID,
		"usr-action",
		"core.network.profile.save",
		"Save network profile",
		"sensitive",
		json.RawMessage(`{"interface":"eth0","method":"dhcp"}`),
		now.Add(4*time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	rejected, err := store.RejectAIToolAction(ctx, rejectable.ID, conversation.ID, "usr-action", now.Add(5*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Status != "rejected" {
		t.Fatalf("rejected status = %q", rejected.Status)
	}
	if _, err := store.RejectAIToolAction(ctx, rejectable.ID, conversation.ID, "usr-action", now.Add(6*time.Second)); !errors.Is(err, ErrAIToolActionNotPending) {
		t.Fatalf("second reject error = %v", err)
	}
}
