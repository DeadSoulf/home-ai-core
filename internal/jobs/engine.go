package jobs

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/events"
	"github.com/DeadSoulf/home-ai-core/internal/state"
)

type Store interface {
	CreateJob(context.Context, state.JobRecord) error
	Job(context.Context, string) (state.JobRecord, error)
	ListJobs(context.Context, string, int) ([]state.JobRecord, error)
	ClaimNextJob(context.Context, string, time.Time) (state.JobRecord, error)
	UpdateJobProgress(context.Context, string, int, string) error
	CompleteJob(context.Context, string, map[string]any, time.Time) error
	FailJob(context.Context, string, string, string, time.Time) error
	MarkJobCancelled(context.Context, string, string, time.Time) error
	RequestJobCancel(context.Context, string, time.Time) (state.JobRecord, error)
	RecoverInterruptedJobs(context.Context, string) (int64, error)
}

type EventPublisher interface {
	Publish(context.Context, events.Input) (state.EventRecord, error)
}

type Handler func(context.Context, state.JobRecord, Reporter) (map[string]any, error)

type Reporter interface {
	Progress(context.Context, int, string) error
}

type Service struct {
	nodeID  string
	store   Store
	events  EventPublisher
	workers int
	now     func() time.Time

	mu       sync.RWMutex
	handlers map[string]Handler
	active   map[string]context.CancelFunc
	wake     chan struct{}
}

func New(nodeID string, store Store, publisher EventPublisher, workers int) *Service {
	if workers <= 0 {
		workers = 2
	}
	return &Service{
		nodeID:   nodeID,
		store:    store,
		events:   publisher,
		workers:  workers,
		now:      time.Now,
		handlers: make(map[string]Handler),
		active:   make(map[string]context.CancelFunc),
		wake:     make(chan struct{}, 1),
	}
}

func (s *Service) Register(jobType string, handler Handler) error {
	if jobType == "" || handler == nil {
		return errors.New("job type and handler are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.handlers[jobType]; exists {
		return fmt.Errorf("job handler %q already registered", jobType)
	}
	s.handlers[jobType] = handler
	return nil
}

func (s *Service) Submit(ctx context.Context, job state.JobRecord) (state.JobRecord, error) {
	if job.Type == "" {
		return state.JobRecord{}, errors.New("job type is required")
	}
	if job.ID == "" {
		id, err := newID("job_")
		if err != nil {
			return state.JobRecord{}, err
		}
		job.ID = id
	}
	job.NodeID = s.nodeID
	job.Status = "queued"
	job.Progress = 0
	job.CreatedAt = s.now().UTC()
	if job.ActorType == "" {
		job.ActorType = "system"
	}

	if err := s.store.CreateJob(ctx, job); err != nil {
		return state.JobRecord{}, err
	}
	s.emit(ctx, job, "job.queued", map[string]any{"status": job.Status})
	s.signal()
	return s.store.Job(ctx, job.ID)
}

func (s *Service) Get(ctx context.Context, id string) (state.JobRecord, error) {
	return s.store.Job(ctx, id)
}

func (s *Service) List(ctx context.Context, status string, limit int) ([]state.JobRecord, error) {
	return s.store.ListJobs(ctx, status, limit)
}

func (s *Service) Cancel(ctx context.Context, id string) (state.JobRecord, error) {
	job, err := s.store.RequestJobCancel(ctx, id, s.now())
	if err != nil {
		return state.JobRecord{}, err
	}

	s.mu.RLock()
	cancel := s.active[id]
	s.mu.RUnlock()
	if cancel != nil {
		cancel()
	}

	s.emit(ctx, job, "job.cancel_requested", map[string]any{"status": job.Status})
	return job, nil
}

func (s *Service) Run(ctx context.Context) error {
	if _, err := s.store.RecoverInterruptedJobs(ctx, s.nodeID); err != nil {
		return err
	}

	var wg sync.WaitGroup
	for i := 0; i < s.workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.worker(ctx)
		}()
	}
	<-ctx.Done()
	wg.Wait()
	return nil
}

func (s *Service) worker(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		if s.runOne(ctx) {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-s.wake:
		case <-ticker.C:
		}
	}
}

func (s *Service) runOne(ctx context.Context) bool {
	job, err := s.store.ClaimNextJob(ctx, s.nodeID, s.now())
	if errors.Is(err, state.ErrJobNotFound) {
		return false
	}
	if err != nil {
		return false
	}

	s.mu.RLock()
	handler := s.handlers[job.Type]
	s.mu.RUnlock()

	if handler == nil {
		_ = s.store.FailJob(ctx, job.ID, "handler_not_registered", "job handler is not registered", s.now())
		s.emit(ctx, job, "job.failed", map[string]any{"code": "handler_not_registered"})
		return true
	}

	jobCtx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.active[job.ID] = cancel
	s.mu.Unlock()

	reporter := reporter{service: s, job: job}
	s.emit(ctx, job, "job.started", map[string]any{"status": "running"})
	result, runErr := handler(jobCtx, job, reporter)

	s.mu.Lock()
	delete(s.active, job.ID)
	s.mu.Unlock()
	cancel()

	current, getErr := s.store.Job(context.WithoutCancel(ctx), job.ID)
	if getErr == nil && current.Status == "cancel_requested" {
		_ = s.store.MarkJobCancelled(context.WithoutCancel(ctx), job.ID, "cancelled", s.now())
		s.emit(context.WithoutCancel(ctx), current, "job.cancelled", map[string]any{"status": "cancelled"})
		return true
	}

	if runErr != nil {
		code := "job_failed"
		message := runErr.Error()
		if errors.Is(runErr, context.Canceled) {
			code = "cancelled"
			message = "job cancelled"
		}
		_ = s.store.FailJob(context.WithoutCancel(ctx), job.ID, code, message, s.now())
		s.emit(context.WithoutCancel(ctx), job, "job.failed", map[string]any{"code": code, "message": message})
		return true
	}

	_ = s.store.CompleteJob(context.WithoutCancel(ctx), job.ID, result, s.now())
	s.emit(context.WithoutCancel(ctx), job, "job.succeeded", map[string]any{"status": "succeeded"})
	return true
}

func (s *Service) emit(ctx context.Context, job state.JobRecord, eventType string, data map[string]any) {
	if s.events == nil {
		return
	}
	_, _ = s.events.Publish(ctx, events.Input{
		Type:          eventType,
		Component:     "jobs",
		ActorType:     job.ActorType,
		ActorID:       job.ActorID,
		RequestID:     job.RequestID,
		CorrelationID: job.CorrelationID,
		JobID:         job.ID,
		Data:          data,
	})
}

func (s *Service) signal() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

type reporter struct {
	service *Service
	job     state.JobRecord
}

func (r reporter) Progress(ctx context.Context, progress int, message string) error {
	if err := r.service.store.UpdateJobProgress(ctx, r.job.ID, progress, message); err != nil {
		return err
	}
	r.service.emit(ctx, r.job, "job.progress", map[string]any{
		"progress": progress,
		"message":  message,
	})
	return nil
}

func newID(prefix string) (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate job id: %w", err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(raw), nil
}
