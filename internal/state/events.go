package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

type EventRecord struct {
	Cursor        int64          `json:"cursor"`
	ID            string         `json:"id"`
	NodeID        string         `json:"node_id"`
	Component     string         `json:"component"`
	Type          string         `json:"type"`
	ActorType     string         `json:"actor_type,omitempty"`
	ActorID       string         `json:"actor_id,omitempty"`
	RequestID     string         `json:"request_id,omitempty"`
	CorrelationID string         `json:"correlation_id,omitempty"`
	JobID         string         `json:"job_id,omitempty"`
	OccurredAt    time.Time      `json:"occurred_at"`
	Data          map[string]any `json:"data"`
}

func (s *Store) AppendEvent(ctx context.Context, event EventRecord) (EventRecord, error) {
	dataJSON, err := marshalObject(event.Data)
	if err != nil {
		return EventRecord{}, fmt.Errorf("encode event data: %w", err)
	}

	result, err := s.db.ExecContext(ctx, `
		INSERT INTO event_log(
			id, node_id, component, type,
			actor_type, actor_id, request_id, correlation_id, job_id,
			occurred_at, data_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		event.ID,
		event.NodeID,
		event.Component,
		event.Type,
		nullIfEmpty(event.ActorType),
		nullIfEmpty(event.ActorID),
		nullIfEmpty(event.RequestID),
		nullIfEmpty(event.CorrelationID),
		nullIfEmpty(event.JobID),
		event.OccurredAt.UTC().Format(time.RFC3339Nano),
		dataJSON,
	)
	if err != nil {
		return EventRecord{}, fmt.Errorf("append event: %w", err)
	}

	cursor, err := result.LastInsertId()
	if err != nil {
		return EventRecord{}, fmt.Errorf("read event cursor: %w", err)
	}
	event.Cursor = cursor
	return event, nil
}

func (s *Store) ListEvents(ctx context.Context, after int64, limit int) ([]EventRecord, error) {
	if after < 0 {
		after = 0
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT
			cursor, id, node_id, component, type,
			COALESCE(actor_type, ''), COALESCE(actor_id, ''),
			COALESCE(request_id, ''), COALESCE(correlation_id, ''),
			COALESCE(job_id, ''), occurred_at, data_json
		FROM event_log
		WHERE cursor > ?
		ORDER BY cursor ASC
		LIMIT ?
	`, after, limit)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer rows.Close()

	events := make([]EventRecord, 0)
	for rows.Next() {
		var event EventRecord
		var occurredAt, dataJSON string
		if err := rows.Scan(
			&event.Cursor,
			&event.ID,
			&event.NodeID,
			&event.Component,
			&event.Type,
			&event.ActorType,
			&event.ActorID,
			&event.RequestID,
			&event.CorrelationID,
			&event.JobID,
			&occurredAt,
			&dataJSON,
		); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		event.OccurredAt, err = time.Parse(time.RFC3339Nano, occurredAt)
		if err != nil {
			return nil, fmt.Errorf("parse event timestamp: %w", err)
		}
		if err := json.Unmarshal([]byte(dataJSON), &event.Data); err != nil {
			return nil, fmt.Errorf("decode event data: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate events: %w", err)
	}
	return events, nil
}

func (s *Store) Event(ctx context.Context, id string) (EventRecord, error) {
	var event EventRecord
	var occurredAt, dataJSON string

	err := s.db.QueryRowContext(ctx, `
		SELECT
			cursor, id, node_id, component, type,
			COALESCE(actor_type, ''), COALESCE(actor_id, ''),
			COALESCE(request_id, ''), COALESCE(correlation_id, ''),
			COALESCE(job_id, ''), occurred_at, data_json
		FROM event_log
		WHERE id = ?
	`, id).Scan(
		&event.Cursor,
		&event.ID,
		&event.NodeID,
		&event.Component,
		&event.Type,
		&event.ActorType,
		&event.ActorID,
		&event.RequestID,
		&event.CorrelationID,
		&event.JobID,
		&occurredAt,
		&dataJSON,
	)
	if err != nil {
		return EventRecord{}, err
	}
	event.OccurredAt, err = time.Parse(time.RFC3339Nano, occurredAt)
	if err != nil {
		return EventRecord{}, fmt.Errorf("parse event timestamp: %w", err)
	}
	if err := json.Unmarshal([]byte(dataJSON), &event.Data); err != nil {
		return EventRecord{}, fmt.Errorf("decode event data: %w", err)
	}
	return event, nil
}

func (s *Store) LatestEventCursor(ctx context.Context) (int64, error) {
	var cursor sql.NullInt64
	if err := s.db.QueryRowContext(ctx, "SELECT MAX(cursor) FROM event_log").Scan(&cursor); err != nil {
		return 0, fmt.Errorf("read latest event cursor: %w", err)
	}
	if !cursor.Valid {
		return 0, nil
	}
	return cursor.Int64, nil
}
