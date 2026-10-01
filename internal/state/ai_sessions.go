package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var ErrAISessionNotFound = errors.New("AI session not found")

type AISessionRecord struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Provider  string    `json:"provider"`
	Model     string    `json:"model"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type AIMessageRecord struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Store) CreateAISession(ctx context.Context, record AISessionRecord) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO ai_sessions(id, user_id, provider, model, title, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`,
		record.ID,
		record.UserID,
		record.Provider,
		record.Model,
		record.Title,
		record.CreatedAt.UTC().Format(time.RFC3339Nano),
		record.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("create AI session: %w", err)
	}
	return nil
}

func (s *Store) AISession(ctx context.Context, id, userID string) (AISessionRecord, error) {
	var record AISessionRecord
	var createdAt, updatedAt string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, user_id, provider, model, title, created_at, updated_at
		FROM ai_sessions
		WHERE id = ? AND user_id = ?
	`, id, userID).Scan(
		&record.ID,
		&record.UserID,
		&record.Provider,
		&record.Model,
		&record.Title,
		&createdAt,
		&updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return AISessionRecord{}, ErrAISessionNotFound
	}
	if err != nil {
		return AISessionRecord{}, fmt.Errorf("read AI session: %w", err)
	}
	var parseErr error
	if record.CreatedAt, parseErr = time.Parse(time.RFC3339Nano, createdAt); parseErr != nil {
		return AISessionRecord{}, fmt.Errorf("parse AI session created_at: %w", parseErr)
	}
	if record.UpdatedAt, parseErr = time.Parse(time.RFC3339Nano, updatedAt); parseErr != nil {
		return AISessionRecord{}, fmt.Errorf("parse AI session updated_at: %w", parseErr)
	}
	return record, nil
}

func (s *Store) ListAISessions(ctx context.Context, userID string, limit int) ([]AISessionRecord, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, user_id, provider, model, title, created_at, updated_at
		FROM ai_sessions
		WHERE user_id = ?
		ORDER BY updated_at DESC
		LIMIT ?
	`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list AI sessions: %w", err)
	}
	defer rows.Close()

	result := make([]AISessionRecord, 0)
	for rows.Next() {
		var record AISessionRecord
		var createdAt, updatedAt string
		if err := rows.Scan(
			&record.ID,
			&record.UserID,
			&record.Provider,
			&record.Model,
			&record.Title,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan AI session: %w", err)
		}
		var parseErr error
		record.CreatedAt, parseErr = time.Parse(time.RFC3339Nano, createdAt)
		if parseErr != nil {
			return nil, fmt.Errorf("parse AI session created_at: %w", parseErr)
		}
		record.UpdatedAt, parseErr = time.Parse(time.RFC3339Nano, updatedAt)
		if parseErr != nil {
			return nil, fmt.Errorf("parse AI session updated_at: %w", parseErr)
		}
		result = append(result, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate AI sessions: %w", err)
	}
	return result, nil
}

func (s *Store) AppendAIMessage(ctx context.Context, userID string, record AIMessageRecord) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin AI message append: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, `
		INSERT INTO ai_messages(id, session_id, role, content, created_at)
		SELECT ?, s.id, ?, ?, ?
		FROM ai_sessions s
		WHERE s.id = ? AND s.user_id = ?
	`,
		record.ID,
		record.Role,
		record.Content,
		record.CreatedAt.UTC().Format(time.RFC3339Nano),
		record.SessionID,
		userID,
	)
	if err != nil {
		return fmt.Errorf("append AI message: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read AI message append result: %w", err)
	}
	if affected != 1 {
		return ErrAISessionNotFound
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE ai_sessions
		SET updated_at = ?
		WHERE id = ? AND user_id = ?
	`, record.CreatedAt.UTC().Format(time.RFC3339Nano), record.SessionID, userID); err != nil {
		return fmt.Errorf("touch AI session: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit AI message append: %w", err)
	}
	return nil
}

func (s *Store) ListAIMessages(ctx context.Context, sessionID, userID string, limit int) ([]AIMessageRecord, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	var ownerCount int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM ai_sessions WHERE id = ? AND user_id = ?
	`, sessionID, userID).Scan(&ownerCount); err != nil {
		return nil, fmt.Errorf("check AI session ownership: %w", err)
	}
	if ownerCount != 1 {
		return nil, ErrAISessionNotFound
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, session_id, role, content, created_at
		FROM (
			SELECT id, session_id, role, content, created_at
			FROM ai_messages
			WHERE session_id = ?
			ORDER BY created_at DESC
			LIMIT ?
		)
		ORDER BY created_at ASC
	`, sessionID, limit)
	if err != nil {
		return nil, fmt.Errorf("list AI messages: %w", err)
	}
	defer rows.Close()

	result := make([]AIMessageRecord, 0)
	for rows.Next() {
		var record AIMessageRecord
		var createdAt string
		if err := rows.Scan(&record.ID, &record.SessionID, &record.Role, &record.Content, &createdAt); err != nil {
			return nil, fmt.Errorf("scan AI message: %w", err)
		}
		parsed, parseErr := time.Parse(time.RFC3339Nano, createdAt)
		if parseErr != nil {
			return nil, fmt.Errorf("parse AI message created_at: %w", parseErr)
		}
		record.CreatedAt = parsed
		result = append(result, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate AI messages: %w", err)
	}
	return result, nil
}

func (s *Store) UpdateAISessionTitle(ctx context.Context, id, userID, title string, at time.Time) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE ai_sessions
		SET title = ?, updated_at = ?
		WHERE id = ? AND user_id = ?
	`, title, at.UTC().Format(time.RFC3339Nano), id, userID)
	if err != nil {
		return fmt.Errorf("update AI session title: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read AI session title update: %w", err)
	}
	if affected != 1 {
		return ErrAISessionNotFound
	}
	return nil
}
