package state

import (
	"context"
	"testing"
	"time"
)

func TestUpdateUserProfileStoresPermissionOverridesAndScopes(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	now := time.Date(2026, 9, 30, 20, 0, 0, 0, time.UTC)
	user, err := store.CreateUser(
		ctx,
		"usr_child",
		"child",
		"Child",
		"hash",
		"role_child",
		now,
	)
	if err != nil {
		t.Fatal(err)
	}

	account, err := store.UpdateUserProfile(ctx, user.ID, UserProfileUpdate{
		DisplayName: "Child",
		RoleName:    "child",
		Permissions: []string{
			"security.self.read",
			"security.sessions.manage",
			"files.read",
		},
		ResourcePermissions: []ResourcePermissionRecord{
			{Permission: "files.read", ResourceType: "file_folder", ResourceID: "nsf_school"},
		},
	}, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	if !containsPermission(account.Permissions, "files.read") {
		t.Fatalf("custom allow missing: %#v", account.Permissions)
	}
	if containsPermission(account.Permissions, "system.read") || containsPermission(account.Permissions, "events.read") {
		t.Fatalf("default permissions were not denied: %#v", account.Permissions)
	}
	if len(account.ResourcePermissions) != 1 ||
		account.ResourcePermissions[0].ResourceID != "nsf_school" {
		t.Fatalf("resource permissions = %#v", account.ResourcePermissions)
	}

	reloaded, err := store.UserAccount(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Permissions) != len(account.Permissions) {
		t.Fatalf("permissions changed after reload: %#v != %#v", reloaded.Permissions, account.Permissions)
	}
}

func TestAdministratorUserAccessIncludesEntirePermissionCatalog(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	now := time.Now().UTC()
	user, err := store.CreateUser(
		ctx,
		"usr_admin",
		"admin",
		"Administrator",
		"hash",
		"role_administrator",
		now,
	)
	if err != nil {
		t.Fatal(err)
	}

	account, err := store.UserAccount(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := store.ListPermissions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(account.Permissions) != len(catalog) {
		t.Fatalf("administrator permissions = %d, catalog = %d", len(account.Permissions), len(catalog))
	}
	for _, item := range catalog {
		if !containsPermission(account.Permissions, item.Name) {
			t.Fatalf("administrator missing permission %q", item.Name)
		}
	}
}

func containsPermission(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
