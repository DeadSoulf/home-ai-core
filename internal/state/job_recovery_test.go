package state

import (
	"context"
	"testing"
	"time"
)

func TestRecoveryDoesNotReplayCancelledOrUnsafeJobs(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.EnsureNode(ctx, "node", "test"); err != nil {
		t.Fatal(err)
	}
	cases := []struct{ id, typ, status, want string }{{"install", "module.install", "running", "queued"}, {"stop", "module.control", "running", "failed"}, {"format", "storage.format", "running", "failed"}, {"cancel", "module.install", "cancel_requested", "cancelled"}}
	for _, tt := range cases {
		if err := store.CreateJob(ctx, JobRecord{ID: tt.id, NodeID: "node", Type: tt.typ, Status: tt.status, CreatedAt: time.Now(), ActorType: "system"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.RecoverInterruptedJobs(ctx, "node"); err != nil {
		t.Fatal(err)
	}
	for _, tt := range cases {
		job, err := store.Job(ctx, tt.id)
		if err != nil || job.Status != tt.want {
			t.Fatalf("%s: %#v %v", tt.id, job, err)
		}
	}
}
