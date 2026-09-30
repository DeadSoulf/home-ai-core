package security

import (
	"context"
	"os"
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

func TestBootstrapLoginAuthenticateLogout(t *testing.T) {
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

	tokenBytes, err := os.ReadFile(service.BootstrapTokenPath())
	if err != nil {
		t.Fatalf("read bootstrap token: %v", err)
	}

	result, err := service.Bootstrap(
		ctx,
		string(tokenBytes),
		"Owner",
		"Home Owner",
		"correct horse battery staple",
		RequestContext{RequestID: "req-1", CorrelationID: "corr-1"},
	)
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if result.Actor.Username != "owner" || !result.Actor.Has("system.read") {
		t.Fatalf("unexpected bootstrap actor: %#v", result.Actor)
	}
	if _, err := os.Stat(service.BootstrapTokenPath()); !os.IsNotExist(err) {
		t.Fatal("bootstrap token still exists after initialization")
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
	tokenBytes, err := os.ReadFile(service.BootstrapTokenPath())
	if err != nil {
		t.Fatalf("read bootstrap token: %v", err)
	}
	result, err := service.Bootstrap(
		ctx,
		string(tokenBytes),
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

func TestCreateAndListMemberUser(t *testing.T) {
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
	tokenBytes, err := os.ReadFile(service.BootstrapTokenPath())
	if err != nil {
		t.Fatalf("read bootstrap token: %v", err)
	}
	owner, err := service.Bootstrap(
		ctx,
		string(tokenBytes),
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
		UserProfileInput{
			Username:    "Alice",
			DisplayName: "Alice",
			Password:    "another correct horse battery",
			Profile:     ProfileMember,
		},
		RequestContext{RequestID: "req-create-user"},
	)
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if created.Username != "alice" || len(created.Roles) != 1 || created.Roles[0] != "member" {
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
		t.Fatalf("member Login() error = %v", err)
	}
	if !login.Actor.Has("security.self.read") {
		t.Fatal("member is missing security.self.read")
	}
	if !login.Actor.Has("security.sessions.manage") {
		t.Fatal("member is missing security.sessions.manage")
	}
	if login.Actor.Has("system.read") || login.Actor.Has("storage.manage") || login.Actor.Has("security.users.manage") {
		t.Fatalf("member received privileged permissions: %#v", login.Actor.Permissions)
	}

	if _, err := service.CreateUser(
		ctx,
		owner.Actor,
		UserProfileInput{
			Username:    "ALICE",
			DisplayName: "Duplicate",
			Password:    "another correct horse battery",
			Profile:     ProfileMember,
		},
		RequestContext{},
	); err != ErrUserExists {
		t.Fatalf("duplicate CreateUser() error = %v, want ErrUserExists", err)
	}
}

func TestUserProfilesSupportCustomPermissionsAndScopedResources(t *testing.T) {
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
	tokenBytes, err := os.ReadFile(service.BootstrapTokenPath())
	if err != nil {
		t.Fatalf("read bootstrap token: %v", err)
	}
	owner, err := service.Bootstrap(
		ctx,
		string(tokenBytes),
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
		UserProfileInput{
			Username:    "Kid",
			DisplayName: "Kid",
			Password:    "child account password",
			Profile:     ProfileChild,
			Permissions: []string{
				"security.self.read",
				"security.sessions.manage",
			},
			ResourcePermissions: []PermissionScope{
				{Permission: "files.read", ResourceType: "file_folder", ResourceID: "nsf_media"},
			},
		},
		RequestContext{},
	)
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if created.Profile != ProfileChild {
		t.Fatalf("profile = %q", created.Profile)
	}
	if containsString(created.Permissions, "system.read") {
		t.Fatalf("custom child permissions unexpectedly include system.read: %#v", created.Permissions)
	}
	if len(created.ResourcePermissions) != 1 || created.ResourcePermissions[0].ResourceID != "nsf_media" {
		t.Fatalf("resource permissions = %#v", created.ResourcePermissions)
	}

	login, err := service.Login(ctx, "kid", "child account password", RequestContext{})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if login.Actor.Has("system.read") {
		t.Fatalf("custom permission deny was not effective: %#v", login.Actor.Permissions)
	}
	if !login.Actor.Allows("files.read", "file_folder", "nsf_media") {
		t.Fatalf("scoped file permission missing: %#v", login.Actor.ResourcePermissions)
	}
	if login.Actor.Allows("files.write", "file_folder", "nsf_media") {
		t.Fatal("unexpected scoped file write permission")
	}
}

func TestAdministratorProfileHasDynamicFullAccess(t *testing.T) {
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
	tokenBytes, err := os.ReadFile(service.BootstrapTokenPath())
	if err != nil {
		t.Fatalf("read bootstrap token: %v", err)
	}
	owner, err := service.Bootstrap(
		ctx,
		string(tokenBytes),
		"Owner",
		"Home Owner",
		"correct horse battery staple",
		RequestContext{},
	)
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}

	admin, err := service.CreateUser(
		ctx,
		owner.Actor,
		UserProfileInput{
			Username:    "Admin2",
			DisplayName: "Second admin",
			Password:    "administrator account password",
			Profile:     ProfileAdministrator,
			Permissions: []string{},
		},
		RequestContext{},
	)
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	for _, permission := range []string{"security.users.manage", "storage.manage", "files.manage", "network.manage", "updates.manage"} {
		if !containsString(admin.Permissions, permission) {
			t.Fatalf("administrator missing %q: %#v", permission, admin.Permissions)
		}
	}

	login, err := service.Login(ctx, "admin2", "administrator account password", RequestContext{})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if !login.Actor.Has("security.roles.manage") || !login.Actor.Has("files.manage") {
		t.Fatalf("administrator does not have full access: %#v", login.Actor.Permissions)
	}
}

func TestOwnerCannotBeDisabledOrDemoted(t *testing.T) {
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
	tokenBytes, err := os.ReadFile(service.BootstrapTokenPath())
	if err != nil {
		t.Fatalf("read bootstrap token: %v", err)
	}
	owner, err := service.Bootstrap(
		ctx,
		string(tokenBytes),
		"Owner",
		"Home Owner",
		"correct horse battery staple",
		RequestContext{},
	)
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}

	_, err = service.UpdateUser(ctx, owner.Actor, owner.Actor.ID, UserProfileInput{
		DisplayName: "Home Owner",
		Profile:     ProfileGuest,
		Disabled:    true,
	}, RequestContext{})
	if !errors.Is(err, ErrOwnerImmutable) {
		t.Fatalf("UpdateUser() error = %v, want ErrOwnerImmutable", err)
	}
}
