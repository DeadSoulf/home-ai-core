package security

import (
	"context"
	"errors"
	"testing"

	"github.com/DeadSoulf/home-ai-core/internal/state"
)

func TestPasswordHashAndVerify(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	ok, err := VerifyPassword(hash, "correct horse battery staple")
	if err != nil {
		t.Fatalf("VerifyPassword() error = %v", err)
	}
	if !ok {
		t.Fatal("password did not verify")
	}
	ok, err = VerifyPassword(hash, "wrong password")
	if err != nil {
		t.Fatalf("wrong VerifyPassword() error = %v", err)
	}
	if ok {
		t.Fatal("wrong password verified")
	}
}

func TestFirstOwnerLoginAuthenticateLogout(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	store, err := state.Open(ctx, dir)
	if err != nil {
		t.Fatalf("state.Open() error = %v", err)
	}
	defer store.Close()

	service, err := New(ctx, store, dir)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := service.Bootstrap(
		ctx,
		"Owner",
		"Home Owner",
		"correct horse battery staple",
		RequestContext{RequestID: "req-1", CorrelationID: "corr-1"},
	)
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if result.Actor.Username != "owner" || !result.Actor.Has("system.read") {
		t.Fatalf("unexpected owner actor: %#v", result.Actor)
	}

	actor, err := service.Authenticate(ctx, result.Token)
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if actor.ID != result.Actor.ID {
		t.Fatalf("actor ID = %q", actor.ID)
	}
	if !actor.ValidCSRF(result.CSRFToken) {
		t.Fatal("CSRF token did not validate")
	}

	login, err := service.Login(
		ctx,
		"owner",
		"correct horse battery staple",
		RequestContext{RequestID: "req-2"},
	)
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if login.Token == "" || login.CSRFToken == "" {
		t.Fatal("login secrets are empty")
	}

	if _, err := service.Login(ctx, "owner", "wrong password", RequestContext{}); err != ErrInvalidCredentials {
		t.Fatalf("wrong password error = %v", err)
	}

	if err := service.Logout(ctx, actor, RequestContext{RequestID: "req-3"}); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if _, err := service.Authenticate(ctx, result.Token); err != ErrUnauthorized {
		t.Fatalf("revoked session authentication error = %v", err)
	}

	audit, err := service.ListAudit(ctx, 100)
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	if len(audit) < 4 {
		t.Fatalf("audit records = %d, want at least 4", len(audit))
	}
}

func TestActorAllowsScopedPermission(t *testing.T) {
	actor := Actor{
		ResourcePermissions: []PermissionScope{
			{Permission: "files.read", ResourceType: "folder", ResourceID: "folder_alice"},
		},
	}

	if !actor.Allows("files.read", "folder", "folder_alice") {
		t.Fatal("exact scoped permission was not allowed")
	}
	if actor.Allows("files.read", "folder", "folder_bob") {
		t.Fatal("scope leaked to another resource")
	}
	if actor.Allows("files.write", "folder", "folder_alice") {
		t.Fatal("scope leaked to another permission")
	}

	actor.Permissions = []string{"files.read"}
	if !actor.Allows("files.read", "folder", "folder_bob") {
		t.Fatal("global permission should allow every resource of that operation")
	}
}

func TestScopedPermissionLoadedFromSession(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	store, err := state.Open(ctx, dir)
	if err != nil {
		t.Fatalf("state.Open() error = %v", err)
	}
	defer store.Close()

	service, err := New(ctx, store, dir)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	result, err := service.Bootstrap(
		ctx,
		"Owner",
		"Home Owner",
		"correct horse battery staple",
		RequestContext{},
	)
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}

	if err := store.GrantUserResourcePermission(
		ctx,
		result.Actor.ID,
		"system.read",
		"room",
		"room_living",
	); err != nil {
		t.Fatalf("GrantUserResourcePermission() error = %v", err)
	}

	actor, err := service.Authenticate(ctx, result.Token)
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if len(actor.ResourcePermissions) != 1 {
		t.Fatalf("resource permissions = %#v, want one grant", actor.ResourcePermissions)
	}
	scope := actor.ResourcePermissions[0]
	if scope.Permission != "system.read" || scope.ResourceType != "room" || scope.ResourceID != "room_living" {
		t.Fatalf("unexpected resource permission: %#v", scope)
	}
}

func TestCreateAndListFriendUser(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	store, err := state.Open(ctx, dir)
	if err != nil {
		t.Fatalf("state.Open() error = %v", err)
	}
	defer store.Close()

	service, err := New(ctx, store, dir)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	owner, err := service.Bootstrap(
		ctx,
		"Owner",
		"Home Owner",
		"correct horse battery staple",
		RequestContext{},
	)
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}

	created, err := service.CreateUser(
		ctx,
		owner.Actor,
		"Alice",
		"Alice",
		"another correct horse battery",
		RequestContext{RequestID: "req-create-user"},
	)
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if created.Username != "alice" || len(created.Roles) != 1 || created.Roles[0] != "friend" {
		t.Fatalf("unexpected created user: %#v", created)
	}

	users, err := service.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers() error = %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("users = %d, want 2", len(users))
	}

	login, err := service.Login(ctx, "alice", "another correct horse battery", RequestContext{})
	if err != nil {
		t.Fatalf("friend Login() error = %v", err)
	}
	if !login.Actor.Has("security.self.read") {
		t.Fatal("friend is missing security.self.read")
	}
	if !login.Actor.Has("security.sessions.manage") {
		t.Fatal("friend is missing security.sessions.manage")
	}
	if login.Actor.Has("system.read") || login.Actor.Has("storage.manage") || login.Actor.Has("security.users.manage") {
		t.Fatalf("friend received privileged permissions: %#v", login.Actor.Permissions)
	}

	if _, err := service.CreateUser(
		ctx,
		owner.Actor,
		"ALICE",
		"Duplicate",
		"another correct horse battery",
		RequestContext{},
	); err != ErrUserExists {
		t.Fatalf("duplicate CreateUser() error = %v, want ErrUserExists", err)
	}
}

func TestUnifiedUserAccessCanBeCustomized(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	store, err := state.Open(ctx, dir)
	if err != nil {
		t.Fatalf("state.Open() error = %v", err)
	}
	defer store.Close()

	service, err := New(ctx, store, dir)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	admin, err := service.Bootstrap(
		ctx,
		"Owner",
		"Home Owner",
		"correct horse battery staple",
		RequestContext{},
	)
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}

	now := service.now().UTC()
	pool, err := store.CreateNASPool(ctx, "Main", "/srv/home-ai/main", "/dev/sdb1", "uuid-main", admin.Actor.ID, now)
	if err != nil {
		t.Fatalf("CreateNASPool() error = %v", err)
	}
	folder, err := store.CreateNASFolder(ctx, pool.ID, "Family", "shared", "", admin.Actor.ID, now)
	if err != nil {
		t.Fatalf("CreateNASFolder() error = %v", err)
	}

	created, err := service.CreateUserWithAccess(
		ctx,
		admin.Actor,
		"Parent1",
		"Parent One",
		"another correct horse battery",
		UserAccessInput{
			Profile:     ProfileParent,
			Permissions: []string{"system.read", "network.read"},
			ResourcePermissions: []PermissionScope{
				{Permission: "files.read", ResourceType: "file_folder", ResourceID: folder.ID},
			},
		},
		RequestContext{},
	)
	if err != nil {
		t.Fatalf("CreateUserWithAccess() error = %v", err)
	}
	if created.Profile != ProfileParent {
		t.Fatalf("profile = %q, want parent", created.Profile)
	}
	if !containsString(created.Permissions, "system.read") || !containsString(created.Permissions, "network.read") {
		t.Fatalf("created permissions = %#v", created.Permissions)
	}

	login, err := service.Login(ctx, "parent1", "another correct horse battery", RequestContext{})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if !login.Actor.Has("system.read") || !login.Actor.Has("network.read") {
		t.Fatalf("actor permissions = %#v", login.Actor.Permissions)
	}
	if !login.Actor.Allows("files.read", "file_folder", folder.ID) {
		t.Fatal("scoped file permission was not effective")
	}
	if login.Actor.Has("network.manage") {
		t.Fatal("custom parent access unexpectedly includes network.manage")
	}

	if _, err := service.UpdateUserAccess(
		ctx,
		login.Actor,
		created.ID,
		UserAccessInput{Profile: ProfileAdministrator},
		RequestContext{},
	); !errors.Is(err, ErrAdministratorRequired) {
		t.Fatalf("non-admin self-escalation error = %v, want ErrAdministratorRequired", err)
	}

	if _, err := service.UpdateUserAccess(
		ctx,
		admin.Actor,
		created.ID,
		UserAccessInput{
			Profile:     ProfileParent,
			Permissions: []string{"security.users.manage"},
		},
		RequestContext{},
	); err == nil {
		t.Fatal("administrator-only permission was assigned to a non-admin profile")
	}

	updated, err := service.UpdateUserAccess(
		ctx,
		admin.Actor,
		created.ID,
		UserAccessInput{
			Profile:     ProfileChild,
			Permissions: []string{"system.read"},
		},
		RequestContext{},
	)
	if err != nil {
		t.Fatalf("UpdateUserAccess() error = %v", err)
	}
	if updated.Profile != ProfileChild {
		t.Fatalf("updated profile = %q, want child", updated.Profile)
	}
	if containsString(updated.Permissions, "network.read") {
		t.Fatalf("network.read remained after replacement: %#v", updated.Permissions)
	}
	if len(updated.ResourcePermissions) != 0 {
		t.Fatalf("resource permissions remained after replacement: %#v", updated.ResourcePermissions)
	}

	actor, err := service.Authenticate(ctx, login.Token)
	if err != nil {
		t.Fatalf("Authenticate() after access update error = %v", err)
	}
	if actor.Has("network.read") {
		t.Fatal("existing session did not pick up the access change")
	}
	if actor.Allows("files.read", "file_folder", folder.ID) {
		t.Fatal("existing session retained removed resource access")
	}
}

func containsString(values []string, value string) bool {
	for _, current := range values {
		if current == value {
			return true
		}
	}
	return false
}
