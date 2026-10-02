package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var ErrAIConversationNotFound = errors.New("AI conversation not found")

type AIConversationRecord struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type AIMessageRecord struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	Role           string    `json:"role"`
	Content        string    `json:"content"`
	CreatedAt      time.Time `json:"created_at"`
}

func (s *Store) CreateAIConversation(
	ctx context.Context,
	id, userID, title string,
	now time.Time,
) (AIConversationRecord, error) {
	timestamp := now.UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO ai_conversations(id, user_id, title, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, id, userID, title, timestamp, timestamp)
	if err != nil {
		return AIConversationRecord{}, fmt.Errorf("create AI conversation: %w", err)
	}
	return AIConversationRecord{
		ID: id, UserID: userID, Title: title, CreatedAt: now.UTC(), UpdatedAt: now.UTC(),
	}, nil
}

func (s *Store) ListAIConversations(
	ctx context.Context,
	userID string,
	limit int,
) ([]AIConversationRecord, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, user_id, title, created_at, updated_at
		FROM ai_conversations
		WHERE user_id = ?
		ORDER BY updated_at DESC
		LIMIT ?
	`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list AI conversations: %w", err)
	}
	defer rows.Close()

	result := make([]AIConversationRecord, 0)
	for rows.Next() {
		record, err := scanAIConversation(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate AI conversations: %w", err)
	}
	return result, nil
}

func (s *Store) AIConversation(
	ctx context.Context,
	id, userID string,
) (AIConversationRecord, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, user_id, title, created_at, updated_at
		FROM ai_conversations
		WHERE id = ? AND user_id = ?
	`, id, userID)
	record, err := scanAIConversation(row)
	if errors.Is(err, sql.ErrNoRows) {
		return AIConversationRecord{}, ErrAIConversationNotFound
	}
	return record, err
}

func (s *Store) UpdateAIConversationTitle(
	ctx context.Context,
	id, userID, title string,
	now time.Time,
) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE ai_conversations
		SET title = ?, updated_at = ?
		WHERE id = ? AND user_id = ?
	`, title, now.UTC().Format(time.RFC3339Nano), id, userID)
	if err != nil {
		return fmt.Errorf("update AI conversation title: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrAIConversationNotFound
	}
	return nil
}

func (s *Store) AppendAIMessage(
	ctx context.Context,
	id, conversationID, userID, role, content string,
	now time.Time,
) (AIMessageRecord, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AIMessageRecord{}, fmt.Errorf("begin AI message append: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var count int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM ai_conversations WHERE id = ? AND user_id = ?
	`, conversationID, userID).Scan(&count); err != nil {
		return AIMessageRecord{}, fmt.Errorf("verify AI conversation owner: %w", err)
	}
	if count != 1 {
		return AIMessageRecord{}, ErrAIConversationNotFound
	}

	timestamp := now.UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ai_messages(id, conversation_id, role, content, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, id, conversationID, role, content, timestamp); err != nil {
		return AIMessageRecord{}, fmt.Errorf("append AI message: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE ai_conversations SET updated_at = ? WHERE id = ?
	`, timestamp, conversationID); err != nil {
		return AIMessageRecord{}, fmt.Errorf("touch AI conversation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return AIMessageRecord{}, fmt.Errorf("commit AI message append: %w", err)
	}

	return AIMessageRecord{
		ID: id, ConversationID: conversationID, Role: role, Content: content, CreatedAt: now.UTC(),
	}, nil
}

func (s *Store) ListAIMessages(
	ctx context.Context,
	conversationID, userID string,
	limit int,
) ([]AIMessageRecord, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM ai_conversations WHERE id = ? AND user_id = ?
	`, conversationID, userID).Scan(&count); err != nil {
		return nil, fmt.Errorf("verify AI conversation owner: %w", err)
	}
	if count != 1 {
		return nil, ErrAIConversationNotFound
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, conversation_id, role, content, created_at
		FROM (
			SELECT id, conversation_id, role, content, created_at
			FROM ai_messages
			WHERE conversation_id = ?
			ORDER BY created_at DESC
			LIMIT ?
		)
		ORDER BY created_at ASC
	`, conversationID, limit)
	if err != nil {
		return nil, fmt.Errorf("list AI messages: %w", err)
	}
	defer rows.Close()

	result := make([]AIMessageRecord, 0)
	for rows.Next() {
		var record AIMessageRecord
		var createdAt string
		if err := rows.Scan(
			&record.ID,
			&record.ConversationID,
			&record.Role,
			&record.Content,
			&createdAt,
		); err != nil {
			return nil, fmt.Errorf("scan AI message: %w", err)
		}
		record.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse AI message created_at: %w", err)
		}
		result = append(result, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate AI messages: %w", err)
	}
	return result, nil
}

type aiConversationScanner interface {
	Scan(dest ...any) error
}

func scanAIConversation(scanner aiConversationScanner) (AIConversationRecord, error) {
	var record AIConversationRecord
	var createdAt, updatedAt string
	if err := scanner.Scan(
		&record.ID,
		&record.UserID,
		&record.Title,
		&createdAt,
		&updatedAt,
	); err != nil {
		return AIConversationRecord{}, err
	}
	var err error
	record.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return AIConversationRecord{}, fmt.Errorf("parse AI conversation created_at: %w", err)
	}
	record.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return AIConversationRecord{}, fmt.Errorf("parse AI conversation updated_at: %w", err)
	}
	return record, nil
}
