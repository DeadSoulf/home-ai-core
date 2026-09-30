package state

import (
	"context"
	"testing"
	"time"
)

func TestNASFolderGrants(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	owner, err := store.CreateOwner(
		ctx,
		"usr_owner",
		"owner",
		"Owner",
		"hash-owner",
		now,
	)
	if err != nil {
		t.Fatalf("CreateOwner() error = %v", err)
	}
	member, err := store.CreateUser(
		ctx,
		"usr_member",
		"member",
		"Member",
		"hash-member",
		"role_member",
		now,
	)
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	pool, err := store.CreateNASPool(ctx, "Main Storage", "/srv/home-ai/main", owner.ID, now)
	if err != nil {
		t.Fatalf("CreateNASPool() error = %v", err)
	}
	if pool.RootPath != "/srv/home-ai/main" {
		t.Fatalf("pool root = %q", pool.RootPath)
	}

	privateFolder, err := store.CreateNASFolder(
		ctx,
		pool.ID,
		"My Files",
		"private",
		member.ID,
		owner.ID,
		now,
	)
	if err != nil {
		t.Fatalf("CreateNASFolder(private) error = %v", err)
	}
	if privateFolder.OwnerUserID != member.ID {
		t.Fatalf("private owner = %q", privateFolder.OwnerUserID)
	}
	if privateFolder.PoolRoot != "/srv/home-ai/main" {
		t.Fatalf("private pool root = %q", privateFolder.PoolRoot)
	}

	_, _, privateScopes, err := store.userAccess(ctx, member.ID)
	if err != nil {
		t.Fatalf("userAccess(member) error = %v", err)
	}
	assertResourcePermission(t, privateScopes, "files.read", "file_folder", privateFolder.ID)
	assertResourcePermission(t, privateScopes, "files.write", "file_folder", privateFolder.ID)

	sharedFolder, err := store.CreateNASFolder(
		ctx,
		pool.ID,
		"Family",
		"shared",
		"",
		owner.ID,
		now,
	)
	if err != nil {
		t.Fatalf("CreateNASFolder(shared) error = %v", err)
	}
	if sharedFolder.OwnerUserID != "" {
		t.Fatalf("shared owner = %q, want empty", sharedFolder.OwnerUserID)
	}

	_, _, sharedScopes, err := store.userAccess(ctx, member.ID)
	if err != nil {
		t.Fatalf("userAccess(member) after shared folder error = %v", err)
	}
	assertNoResourcePermission(t, sharedScopes, "files.read", "file_folder", sharedFolder.ID)
	assertNoResourcePermission(t, sharedScopes, "files.write", "file_folder", sharedFolder.ID)

	folders, err := store.ListNASFolders(ctx)
	if err != nil {
		t.Fatalf("ListNASFolders() error = %v", err)
	}
	if len(folders) != 2 {
		t.Fatalf("folder count = %d, want 2", len(folders))
	}

	if err := store.DeleteNASFolder(ctx, privateFolder.ID); err != nil {
		t.Fatalf("DeleteNASFolder() error = %v", err)
	}
	_, _, scopesAfterDelete, err := store.userAccess(ctx, member.ID)
	if err != nil {
		t.Fatalf("userAccess(member) after delete error = %v", err)
	}
	for _, scope := range scopesAfterDelete {
		if scope.ResourceType == "file_folder" && scope.ResourceID == privateFolder.ID {
			t.Fatalf("stale private folder grant after delete: %#v", scope)
		}
	}
}

func TestNASPoolValidation(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	now := time.Now().UTC()
	if _, err := store.CreateNASPool(ctx, "Main", "relative/path", "", now); err == nil {
		t.Fatal("relative NAS root path was accepted")
	}
}

func assertResourcePermission(
	t *testing.T,
	records []ResourcePermissionRecord,
	permission, resourceType, resourceID string,
) {
	t.Helper()
	for _, record := range records {
		if record.Permission == permission &&
			record.ResourceType == resourceType &&
			record.ResourceID == resourceID {
			return
		}
	}
	t.Fatalf(
		"missing resource permission %s on %s/%s in %#v",
		permission,
		resourceType,
		resourceID,
		records,
	)
}

func assertNoResourcePermission(
	t *testing.T,
	records []ResourcePermissionRecord,
	permission, resourceType, resourceID string,
) {
	t.Helper()
	for _, record := range records {
		if record.Permission == permission &&
			record.ResourceType == resourceType &&
			record.ResourceID == resourceID {
			t.Fatalf(
				"unexpected resource permission %s on %s/%s in %#v",
				permission,
				resourceType,
				resourceID,
				records,
			)
		}
	}
}
