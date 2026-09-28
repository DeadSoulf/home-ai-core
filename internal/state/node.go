package state

import (
	"context"
	"fmt"
	"time"
)

type Node struct {
	ID         string    `json:"id"`
	Hostname   string    `json:"hostname"`
	CreatedAt  time.Time `json:"created_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
}

func (s *Store) EnsureNode(ctx context.Context, id, hostname string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO nodes(id, hostname, created_at, last_seen_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			hostname = excluded.hostname,
			last_seen_at = excluded.last_seen_at
	`, id, hostname, now, now)
	if err != nil {
		return fmt.Errorf("ensure node record: %w", err)
	}
	return nil
}

func (s *Store) Node(ctx context.Context, id string) (Node, error) {
	var node Node
	var createdAt, lastSeenAt string

	err := s.db.QueryRowContext(ctx, `
		SELECT id, hostname, created_at, last_seen_at
		FROM nodes
		WHERE id = ?
	`, id).Scan(&node.ID, &node.Hostname, &createdAt, &lastSeenAt)
	if err != nil {
		return Node{}, fmt.Errorf("read node record: %w", err)
	}

	var err error
	node.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return Node{}, fmt.Errorf("parse node created_at: %w", err)
	}
	node.LastSeenAt, err = time.Parse(time.RFC3339Nano, lastSeenAt)
	if err != nil {
		return Node{}, fmt.Errorf("parse node last_seen_at: %w", err)
	}

	return node, nil
}
