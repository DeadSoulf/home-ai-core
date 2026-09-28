package events

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/realtime"
	"github.com/DeadSoulf/home-ai-core/internal/state"
)

type Store interface {
	AppendEvent(context.Context, state.EventRecord) (state.EventRecord, error)
	ListEvents(context.Context, int64, int) ([]state.EventRecord, error)
	LatestEventCursor(context.Context) (int64, error)
}

type Input struct {
	Type          string
	Component     string
	ActorType     string
	ActorID       string
	RequestID     string
	CorrelationID string
	JobID         string
	Data          map[string]any
}

type Service struct {
	nodeID string
	store  Store
	hub    *realtime.Hub
	now    func() time.Time
}

func New(nodeID string, store Store, hub *realtime.Hub) *Service {
	return &Service{
		nodeID: nodeID,
		store:  store,
		hub:    hub,
		now:    time.Now,
	}
}

func (s *Service) Publish(ctx context.Context, input Input) (state.EventRecord, error) {
	id, err := newID("evt_")
	if err != nil {
		return state.EventRecord{}, err
	}

	record, err := s.store.AppendEvent(ctx, state.EventRecord{
		ID:            id,
		NodeID:        s.nodeID,
		Component:     defaultComponent(input.Component),
		Type:          input.Type,
		ActorType:     input.ActorType,
		ActorID:       input.ActorID,
		RequestID:     input.RequestID,
		CorrelationID: input.CorrelationID,
		JobID:         input.JobID,
		OccurredAt:    s.now().UTC(),
		Data:          input.Data,
	})
	if err != nil {
		return state.EventRecord{}, err
	}

	s.hub.PublishEvent(realtime.PublishedEvent{
		ID:        record.ID,
		Cursor:    record.Cursor,
		Type:      record.Type,
		Time:      record.OccurredAt,
		Source:    realtime.Source{NodeID: record.NodeID, Component: record.Component},
		RequestID: record.RequestID,
		Data:      record.Data,
	})
	return record, nil
}

func (s *Service) History(ctx context.Context, after int64, limit int) ([]state.EventRecord, error) {
	return s.store.ListEvents(ctx, after, limit)
}

func (s *Service) LatestCursor(ctx context.Context) (int64, error) {
	return s.store.LatestEventCursor(ctx)
}

func defaultComponent(value string) string {
	if value == "" {
		return "core"
	}
	return value
}

func newID(prefix string) (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate event id: %w", err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(raw), nil
}
