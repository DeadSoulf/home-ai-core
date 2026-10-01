package security

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/DeadSoulf/home-ai-core/internal/state"
	"os"
	"strings"
	"testing"
)

func lifecycleFixture(t *testing.T) (context.Context, *Service, AuthResult) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	store, err := state.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service, err := New(ctx, store, dir)
	if err != nil {
		t.Fatal(err)
	}
	token, err := os.ReadFile(service.BootstrapTokenPath())
	if err != nil {
		t.Fatal(err)
	}
	login, err := service.Bootstrap(ctx, string(token), "owner", "Owner", "initial correct password", RequestContext{})
	if err != nil {
		t.Fatal(err)
	}
	return ctx, service, login
}

func TestPasswordChangeRequiresCurrentPasswordAndRevokesEverySession(t *testing.T) {
	ctx, service, owner := lifecycleFixture(t)
	second, err := service.Login(ctx, "owner", "initial correct password", RequestContext{})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ChangePassword(ctx, owner.Actor, "incorrect password", "replacement correct password", RequestContext{}); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong current password: %v", err)
	}
	if _, err := service.Authenticate(ctx, second.Token); err != nil {
		t.Fatal("failed change revoked valid session", err)
	}
	if err := service.ChangePassword(ctx, owner.Actor, "initial correct password", "replacement correct password", RequestContext{}); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{owner.Token, second.Token} {
		if _, err := service.Authenticate(ctx, token); !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("old session remains: %v", err)
		}
	}
	if _, err := service.Login(ctx, "owner", "initial correct password", RequestContext{}); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatal("old password accepted", err)
	}
	if _, err := service.Login(ctx, "owner", "replacement correct password", RequestContext{}); err != nil {
		t.Fatal(err)
	}
	audit, err := service.ListAudit(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(audit)
	if strings.Contains(string(encoded), "initial correct password") || strings.Contains(string(encoded), "replacement correct password") {
		t.Fatal("password leaked to audit")
	}
}

func TestIdentityAndAdministrativeResetPreserveStableOwner(t *testing.T) {
	ctx, service, owner := lifecycleFixture(t)
	user, err := service.CreateUserWithAccess(ctx, owner.Actor, "alice", "Alice", "alice initial password", UserAccessInput{Profile: ProfileFriend}, RequestContext{})
	if err != nil {
		t.Fatal(err)
	}
	login, err := service.Login(ctx, "alice", "alice initial password", RequestContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.UpdateUserIdentity(ctx, login.Actor, user.ID, "owner", "Attack", RequestContext{}); !errors.Is(err, ErrAdministratorRequired) {
		t.Fatalf("non-admin identity: %v", err)
	}
	if _, err := service.UpdateUserIdentity(ctx, owner.Actor, user.ID, "owner", "Alice", RequestContext{}); !errors.Is(err, ErrUserExists) {
		t.Fatalf("duplicate username: %v", err)
	}
	if _, err := service.Authenticate(ctx, login.Token); err != nil {
		t.Fatal("collision revoked session", err)
	}
	renamed, err := service.UpdateUserIdentity(ctx, owner.Actor, user.ID, "alice-new", "Alice New", RequestContext{})
	if err != nil {
		t.Fatal(err)
	}
	if renamed.ID != user.ID {
		t.Fatal("rename changed stable identity")
	}
	if _, err := service.Authenticate(ctx, login.Token); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("rename retained session", err)
	}
	if err := service.ResetUserPassword(ctx, owner.Actor, user.ID, "alice replacement password", RequestContext{}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Login(ctx, "alice-new", "alice replacement password", RequestContext{}); err != nil {
		t.Fatal(err)
	}
	if err := service.ResetUserPassword(ctx, owner.Actor, owner.Actor.ID, "owner replacement password", RequestContext{}); err == nil {
		t.Fatal("own reset bypassed current password")
	}
}
