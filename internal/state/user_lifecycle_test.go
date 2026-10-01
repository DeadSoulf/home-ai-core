package state

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestConcurrentDisableKeepsOneAdministrator(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.CreateOwner(ctx, "usr_owner", "owner", "Owner", "hash", time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateUser(ctx, "usr_other", "other", "Other", "hash", "role_owner", time.Now()); err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	results := make(chan error, 2)
	for _, id := range []string{"usr_owner", "usr_other"} {
		group.Add(1)
		go func(id string) {
			defer group.Done()
			results <- store.SetUserAccess(ctx, id, "role_owner", nil, nil, true, time.Now())
		}(id)
	}
	group.Wait()
	close(results)
	rejected := 0
	for err := range results {
		if errors.Is(err, ErrLastEnabledAdministrator) {
			rejected++
		} else if err != nil {
			t.Fatal(err)
		}
	}
	count, err := store.CountEnabledUsersWithRole(ctx, "role_owner")
	if err != nil || count != 1 || rejected != 1 {
		t.Fatalf("last administrator lost: %d %d %v", count, rejected, err)
	}
}

func TestReplacingUserAccessPreservesPrivateOwnership(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.CreateOwner(ctx, "usr_owner", "owner", "Owner", "hash", time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateUser(ctx, "usr_friend", "friend", "Friend", "hash", "role_member", time.Now()); err != nil {
		t.Fatal(err)
	}
	pool, err := store.CreateNASPool(ctx, "Main", t.TempDir(), "/dev/test", "uuid", "usr_owner", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	folder, err := store.CreateNASFolder(ctx, pool.ID, "Personal", "private", "usr_friend", "usr_owner", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetUserAccess(ctx, "usr_friend", "role_member", nil, nil, false, time.Now()); err != nil {
		t.Fatal(err)
	}
	grants, err := store.NASFolderAccess(ctx, folder.ID)
	if err != nil || len(grants) != 1 || !grants[0].Read || !grants[0].Write {
		t.Fatalf("private owner lost access: %+v %v", grants, err)
	}
}
