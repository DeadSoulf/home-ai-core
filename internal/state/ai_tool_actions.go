package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var (
	ErrAIToolActionNotFound   = errors.New("AI tool action not found")
	ErrAIToolActionNotPending = errors.New("AI tool action is not pending")
)

type AIToolActionRecord struct {
	ID             string          `json:"id"`
	ConversationID string          `json:"conversation_id"`
	UserID         string          `json:"user_id"`
	ToolID         string          `json:"tool_id"`
	ToolName       string          `json:"tool_name"`
	Sensitivity    string          `json:"sensitivity"`
	Input          json.RawMessage `json:"input"`
	Status         string          `json:"status"`
	Result         json.RawMessage `json:"result,omitempty"`
	ErrorCode      string          `json:"error_code,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

func (s *Store) CreateAIToolAction(
	ctx context.Context,
	id, conversationID, userID, toolID, toolName, sensitivity string,
	input json.RawMessage,
	now time.Time,
) (AIToolActionRecord, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM ai_conversations
		WHERE id = ? AND user_id = ? AND closed_at IS NULL
	`, conversationID, userID).Scan(&count); err != nil {
		return AIToolActionRecord{}, fmt.Errorf("verify AI conversation for action: %w", err)
	}
	if count != 1 {
		return AIToolActionRecord{}, ErrAIConversationNotFound
	}
	if len(input) == 0 {
		input = json.RawMessage(`{}`)
	}
	if !json.Valid(input) {
		return AIToolActionRecord{}, errors.New("AI tool action input is not valid JSON")
	}
	timestamp := now.UTC().Format(time.RFC3339Nano)
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO ai_tool_actions(
			id, conversation_id, user_id, tool_id, tool_name, sensitivity,
			input_json, status, created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?)
	`, id, conversationID, userID, toolID, toolName, sensitivity, string(input), timestamp, timestamp); err != nil {
		return AIToolActionRecord{}, fmt.Errorf("create AI tool action: %w", err)
	}
	return s.AIToolAction(ctx, id, conversationID, userID)
}

func (s *Store) ListAIToolActions(
	ctx context.Context,
	conversationID, userID string,
	limit int,
) ([]AIToolActionRecord, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM ai_conversations WHERE id = ? AND user_id = ?
	`, conversationID, userID).Scan(&count); err != nil {
		return nil, fmt.Errorf("verify AI conversation owner for actions: %w", err)
	}
	if count != 1 {
		return nil, ErrAIConversationNotFound
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, conversation_id, user_id, tool_id, tool_name, sensitivity,
		       input_json, status, result_json, error_code, created_at, updated_at
		FROM ai_tool_actions
		WHERE conversation_id = ? AND user_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`, conversationID, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list AI tool actions: %w", err)
	}
	defer rows.Close()

	result := make([]AIToolActionRecord, 0)
	for rows.Next() {
		record, err := scanAIToolAction(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate AI tool actions: %w", err)
	}
	return result, nil
}

func (s *Store) AIToolAction(
	ctx context.Context,
	id, conversationID, userID string,
) (AIToolActionRecord, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, conversation_id, user_id, tool_id, tool_name, sensitivity,
		       input_json, status, result_json, error_code, created_at, updated_at
		FROM ai_tool_actions
		WHERE id = ? AND conversation_id = ? AND user_id = ?
	`, id, conversationID, userID)
	record, err := scanAIToolAction(row)
	if errors.Is(err, sql.ErrNoRows) {
		return AIToolActionRecord{}, ErrAIToolActionNotFound
	}
	return record, err
}

func (s *Store) ClaimAIToolAction(
	ctx context.Context,
	id, conversationID, userID string,
	now time.Time,
) (AIToolActionRecord, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE ai_tool_actions
		SET status = 'executing', updated_at = ?
		WHERE id = ? AND conversation_id = ? AND user_id = ? AND status = 'pending'
	`, now.UTC().Format(time.RFC3339Nano), id, conversationID, userID)
	if err != nil {
		return AIToolActionRecord{}, fmt.Errorf("claim AI tool action: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		record, readErr := s.AIToolAction(ctx, id, conversationID, userID)
		if readErr != nil {
			return AIToolActionRecord{}, readErr
		}
		if record.Status != "pending" {
			return record, ErrAIToolActionNotPending
		}
		return record, ErrAIToolActionNotPending
	}
	return s.AIToolAction(ctx, id, conversationID, userID)
}

func (s *Store) RejectAIToolAction(
	ctx context.Context,
	id, conversationID, userID string,
	now time.Time,
) (AIToolActionRecord, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE ai_tool_actions
		SET status = 'rejected', updated_at = ?
		WHERE id = ? AND conversation_id = ? AND user_id = ? AND status = 'pending'
	`, now.UTC().Format(time.RFC3339Nano), id, conversationID, userID)
	if err != nil {
		return AIToolActionRecord{}, fmt.Errorf("reject AI tool action: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		record, readErr := s.AIToolAction(ctx, id, conversationID, userID)
		if readErr != nil {
			return AIToolActionRecord{}, readErr
		}
		return record, ErrAIToolActionNotPending
	}
	return s.AIToolAction(ctx, id, conversationID, userID)
}

func (s *Store) FinishAIToolAction(
	ctx context.Context,
	id, conversationID, userID, status string,
	resultJSON json.RawMessage,
	errorCode string,
	now time.Time,
) (AIToolActionRecord, error) {
	if status != "executed" && status != "failed" {
		return AIToolActionRecord{}, errors.New("invalid AI tool action final status")
	}
	var resultValue any
	if len(resultJSON) > 0 {
		if !json.Valid(resultJSON) {
			return AIToolActionRecord{}, errors.New("AI tool action result is not valid JSON")
		}
		resultValue = string(resultJSON)
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE ai_tool_actions
		SET status = ?, result_json = ?, error_code = ?, updated_at = ?
		WHERE id = ? AND conversation_id = ? AND user_id = ? AND status = 'executing'
	`, status, resultValue, nullIfEmpty(errorCode), now.UTC().Format(time.RFC3339Nano), id, conversationID, userID)
	if err != nil {
		return AIToolActionRecord{}, fmt.Errorf("finish AI tool action: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		record, readErr := s.AIToolAction(ctx, id, conversationID, userID)
		if readErr != nil {
			return AIToolActionRecord{}, readErr
		}
		return record, ErrAIToolActionNotPending
	}
	return s.AIToolAction(ctx, id, conversationID, userID)
}

type aiToolActionScanner interface {
	Scan(dest ...any) error
}

func scanAIToolAction(scanner aiToolActionScanner) (AIToolActionRecord, error) {
	var record AIToolActionRecord
	var inputJSON string
	var resultJSON, errorCode sql.NullString
	var createdAt, updatedAt string
	if err := scanner.Scan(
		&record.ID,
		&record.ConversationID,
		&record.UserID,
		&record.ToolID,
		&record.ToolName,
		&record.Sensitivity,
		&inputJSON,
		&record.Status,
		&resultJSON,
		&errorCode,
		&createdAt,
		&updatedAt,
	); err != nil {
		return AIToolActionRecord{}, err
	}
	record.Input = json.RawMessage(inputJSON)
	if resultJSON.Valid {
		record.Result = json.RawMessage(resultJSON.String)
	}
	if errorCode.Valid {
		record.ErrorCode = errorCode.String
	}
	var err error
	record.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return AIToolActionRecord{}, fmt.Errorf("parse AI tool action created_at: %w", err)
	}
	record.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return AIToolActionRecord{}, fmt.Errorf("parse AI tool action updated_at: %w", err)
	}
	return record, nil
}
