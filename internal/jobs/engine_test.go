package jobs

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/events"
	"github.com/DeadSoulf/home-ai-core/internal/realtime"
	"github.com/DeadSoulf/home-ai-core/internal/state"
)

func TestJobEngineRunsTypedHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store, err := state.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("state.Open() error = %v", err)
	}
	defer store.Close()

	hub := realtime.New("node-1", slog.New(slog.NewTextHandler(io.Discard, nil)))
	eventService := events.New("node-1", store, hub)
	service := New("node-1", store, eventService, 1)

	if err := service.Register("test.echo", func(ctx context.Context, job state.JobRecord, reporter Reporter) (map[string]any, error) {
		if err := reporter.Progress(ctx, 5000, "halfway"); err != nil {
			return nil, err
		}
		return map[string]any{"echo": job.Input["value"]}, nil
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	go func() { _ = service.Run(ctx) }()

	job, err := service.Submit(ctx, state.JobRecord{
		Type:      "test.echo",
		ActorType: "user",
		ActorID:   "usr-1",
		Input:     map[string]any{"value": "ok"},
	})
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		current, err := service.Get(ctx, job.ID)
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if current.Status == "succeeded" {
			if current.Progress != 10000 || current.Result["echo"] != "ok" {
				t.Fatalf("unexpected completed job: %#v", current)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("job did not complete: %#v", current)
		}
		time.Sleep(20 * time.Millisecond)
	}

	history, err := eventService.History(ctx, 0, 100)
	if err != nil {
		t.Fatalf("History() error = %v", err)
	}
	if len(history) < 4 {
		t.Fatalf("durable job events = %d, want at least 4", len(history))
	}
}

func TestQueuedJobCanBeCancelled(t *testing.T) {
	ctx := context.Background()
	store, err := state.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("state.Open() error = %v", err)
	}
	defer store.Close()

	hub := realtime.New("node-1", slog.New(slog.NewTextHandler(io.Discard, nil)))
	eventService := events.New("node-1", store, hub)
	service := New("node-1", store, eventService, 1)

	job, err := service.Submit(ctx, state.JobRecord{
		Type:      "test.wait",
		ActorType: "user",
	})
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	cancelled, err := service.Cancel(ctx, job.ID)
	if err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if cancelled.Status != "cancelled" {
		t.Fatalf("status = %q, want cancelled", cancelled.Status)
	}
}
