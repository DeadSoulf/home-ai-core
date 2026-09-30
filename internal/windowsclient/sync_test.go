package windowsclient

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/filedata"
)

func TestSyncOnceCopiesNewItemsAndSkipsUnchanged(t *testing.T) {
	source := filepath.Join(t.TempDir(), "documents")
	if err := os.MkdirAll(filepath.Join(source, "empty"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeCopyFile(t, filepath.Join(source, "notes.txt"), "sync bytes")

	remote := newCopyTestServer(t)
	client := remote.client(t)

	first, err := client.SyncOnce(context.Background(), "test", source, "backup/documents", SyncOptions{
		ConflictPolicy: SyncConflictStop,
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Planned != 3 || first.Copied != 3 || first.Unchanged != 0 {
		t.Fatalf("first sync summary = %#v", first)
	}

	before := remote.writeCount()
	second, err := client.SyncOnce(context.Background(), "test", source, "backup/documents", SyncOptions{
		ConflictPolicy: SyncConflictStop,
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.Planned != 3 || second.Copied != 0 || second.Unchanged != 3 {
		t.Fatalf("second sync summary = %#v", second)
	}
	if remote.writeCount() != before {
		t.Fatal("unchanged sync performed remote writes")
	}
}

func TestSyncOnceConflictPolicies(t *testing.T) {
	for _, policy := range []string{SyncConflictStop, SyncConflictSkip, SyncConflictReplaceToTrash} {
		t.Run(policy, func(t *testing.T) {
			source := filepath.Join(t.TempDir(), "source.txt")
			writeCopyFile(t, source, "local!")
			remote := newCopyTestServer(t)
			writeCopyFile(t, filepath.Join(remote.root, "target.txt"), "remote")
			client := remote.client(t)

			summary, err := client.SyncOnce(context.Background(), "test", source, "target.txt", SyncOptions{
				ConflictPolicy: policy,
			})
			switch policy {
			case SyncConflictStop:
				var conflict *DestinationConflictError
				if !errors.As(err, &conflict) {
					t.Fatalf("stop error = %v", err)
				}
				if summary.Conflicts != 1 || summary.Copied != 0 {
					t.Fatalf("stop summary = %#v", summary)
				}
				assertRemoteSyncFile(t, remote.root, "target.txt", "remote")
			case SyncConflictSkip:
				if err != nil {
					t.Fatal(err)
				}
				if summary.Conflicts != 1 || summary.Skipped != 1 || summary.Copied != 0 {
					t.Fatalf("skip summary = %#v", summary)
				}
				assertRemoteSyncFile(t, remote.root, "target.txt", "remote")
			case SyncConflictReplaceToTrash:
				if err != nil {
					t.Fatal(err)
				}
				if summary.Conflicts != 1 || summary.Replaced != 1 || summary.Copied != 1 {
					t.Fatalf("replace summary = %#v", summary)
				}
				assertRemoteSyncFile(t, remote.root, "target.txt", "local!")
				trash, err := filedata.ListTrash(remote.root)
				if err != nil {
					t.Fatal(err)
				}
				if len(trash) != 1 || trash[0].OriginalPath != "target.txt" {
					t.Fatalf("trash = %#v", trash)
				}
			}
		})
	}
}

func TestSyncReplaceRefusesConflictingAncestor(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source.txt")
	writeCopyFile(t, source, "local")
	remote := newCopyTestServer(t)
	writeCopyFile(t, filepath.Join(remote.root, "backup"), "remote-parent")

	_, err := remote.client(t).SyncOnce(context.Background(), "test", source, "backup/source.txt", SyncOptions{
		ConflictPolicy: SyncConflictReplaceToTrash,
	})
	if err == nil {
		t.Fatal("ancestor conflict was replaced")
	}
	assertRemoteSyncFile(t, remote.root, "backup", "remote-parent")
	trash, trashErr := filedata.ListTrash(remote.root)
	if trashErr != nil {
		t.Fatal(trashErr)
	}
	if len(trash) != 0 {
		t.Fatalf("ancestor conflict moved data to trash: %#v", trash)
	}
}

func TestSyncProfilePersistenceAndSchedule(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source.txt")
	writeCopyFile(t, source, "data")
	config := filepath.Join(t.TempDir(), "sync.json")

	profile, err := AddSyncProfile(config, SyncProfileInput{
		ServerURL:      "HTTP://Example.COM:80/",
		Username:       "alice",
		FolderID:       "nsf_test",
		Source:         source,
		Destination:    "backup/source.txt",
		Every:          5 * time.Minute,
		ConflictPolicy: SyncConflictSkip,
	})
	if err != nil {
		t.Fatal(err)
	}
	if profile.ServerURL != "http://example.com" || !profile.Enabled {
		t.Fatalf("profile = %#v", profile)
	}

	profiles, err := LoadSyncProfiles(config)
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 || len(DueSyncProfiles(profiles, time.Now().UTC())) != 1 {
		t.Fatalf("loaded profiles = %#v", profiles)
	}

	attempt := time.Now().UTC().Add(time.Second)
	if err := RecordSyncProfileResult(config, profile.ID, attempt, nil); err != nil {
		t.Fatal(err)
	}
	profiles, err = LoadSyncProfiles(config)
	if err != nil {
		t.Fatal(err)
	}
	if len(DueSyncProfiles(profiles, attempt.Add(4*time.Minute))) != 0 {
		t.Fatal("profile became due before its interval")
	}
	if len(DueSyncProfiles(profiles, attempt.Add(5*time.Minute))) != 1 {
		t.Fatal("profile was not due at its interval")
	}
	if profiles[0].LastSuccessAt == nil || profiles[0].LastError != "" {
		t.Fatalf("result state = %#v", profiles[0])
	}

	if err := SetSyncProfileEnabled(config, profile.ID, false); err != nil {
		t.Fatal(err)
	}
	profiles, err = LoadSyncProfiles(config)
	if err != nil {
		t.Fatal(err)
	}
	if profiles[0].Enabled || len(DueSyncProfiles(profiles, attempt.Add(time.Hour))) != 0 {
		t.Fatal("disabled profile remained due")
	}
	if err := RemoveSyncProfile(config, profile.ID); err != nil {
		t.Fatal(err)
	}
	profiles, err = LoadSyncProfiles(config)
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 0 {
		t.Fatalf("profiles after remove = %#v", profiles)
	}
}

func TestUpdateSyncProfilePreservesState(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source.txt")
	writeCopyFile(t, source, "data")
	nextSource := filepath.Join(t.TempDir(), "next.txt")
	writeCopyFile(t, nextSource, "next")
	config := filepath.Join(t.TempDir(), "sync.json")

	profile, err := AddSyncProfile(config, SyncProfileInput{
		ServerURL:      "http://example.com",
		Username:       "alice",
		FolderID:       "nsf_old",
		Source:         source,
		Destination:    "old/source.txt",
		Every:          5 * time.Minute,
		ConflictPolicy: SyncConflictStop,
	})
	if err != nil {
		t.Fatal(err)
	}
	attempt := time.Now().UTC().Add(time.Second)
	if err := RecordSyncProfileResult(config, profile.ID, attempt, nil); err != nil {
		t.Fatal(err)
	}
	if err := SetSyncProfileEnabled(config, profile.ID, false); err != nil {
		t.Fatal(err)
	}

	updated, err := UpdateSyncProfile(config, profile.ID, SyncProfileInput{
		ServerURL:      "https://home-ai.example",
		Username:       "bob",
		FolderID:       "nsf_new",
		Source:         nextSource,
		Destination:    "new/next.txt",
		Every:          30 * time.Minute,
		ConflictPolicy: SyncConflictReplaceToTrash,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != profile.ID || updated.Enabled {
		t.Fatalf("updated identity/state = %#v", updated)
	}
	if updated.LastSuccessAt == nil || !updated.LastSuccessAt.Equal(attempt) {
		t.Fatalf("updated result history = %#v", updated)
	}
	if updated.CreatedAt != profile.CreatedAt {
		t.Fatalf("created_at changed: %v -> %v", profile.CreatedAt, updated.CreatedAt)
	}
	if updated.ServerURL != "https://home-ai.example" || updated.Username != "bob" ||
		updated.FolderID != "nsf_new" || updated.Source != nextSource ||
		updated.Destination != "new/next.txt" || updated.Interval() != 30*time.Minute ||
		updated.ConflictPolicy != SyncConflictReplaceToTrash {
		t.Fatalf("updated fields = %#v", updated)
	}
}

func TestAddSyncProfileRejectsUnsafeConfiguration(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source.txt")
	writeCopyFile(t, source, "data")
	config := filepath.Join(t.TempDir(), "sync.json")

	for _, input := range []SyncProfileInput{
		{ServerURL: "http://example.com", Username: "alice", FolderID: "nsf", Source: source, Destination: "../escape", Every: time.Minute, ConflictPolicy: SyncConflictStop},
		{ServerURL: "http://example.com", Username: "alice", FolderID: "nsf", Source: source, Destination: "safe", Every: time.Second, ConflictPolicy: SyncConflictStop},
		{ServerURL: "http://example.com", Username: "alice", FolderID: "nsf", Source: source, Destination: "safe", Every: time.Minute, ConflictPolicy: "overwrite"},
	} {
		if _, err := AddSyncProfile(config, input); err == nil {
			t.Fatalf("accepted unsafe sync input: %#v", input)
		}
	}
}

func assertRemoteSyncFile(t *testing.T, root, relative, want string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Fatalf("%s = %q, want %q", relative, data, want)
	}
}
