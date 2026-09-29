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
	ErrJobNotFound = errors.New("job not found")
	ErrJobTerminal = errors.New("job is already terminal")
)

type JobRecord struct {
	ID                string         `json:"id"`
	NodeID            string         `json:"node_id"`
	Type              string         `json:"type"`
	Status            string         `json:"status"`
	Progress          int            `json:"progress"`
	Message           string         `json:"message,omitempty"`
	ActorType         string         `json:"actor_type"`
	ActorID           string         `json:"actor_id,omitempty"`
	RequestID         string         `json:"request_id,omitempty"`
	CorrelationID     string         `json:"correlation_id,omitempty"`
	Input             map[string]any `json:"input,omitempty"`
	Result            map[string]any `json:"result,omitempty"`
	ErrorCode         string         `json:"error_code,omitempty"`
	ErrorMessage      string         `json:"error_message,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
	StartedAt         *time.Time     `json:"started_at,omitempty"`
	CompletedAt       *time.Time     `json:"completed_at,omitempty"`
	CancelRequestedAt *time.Time     `json:"cancel_requested_at,omitempty"`
}

func (s *Store) CreateJob(ctx context.Context, job JobRecord) error {
	inputJSON, err := marshalObject(job.Input)
	if err != nil {
		return fmt.Errorf("encode job input: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO jobs(
			id, node_id, type, status, progress, message,
			actor_type, actor_id, request_id, correlation_id,
			input_json, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		job.ID,
		job.NodeID,
		job.Type,
		job.Status,
		job.Progress,
		job.Message,
		job.ActorType,
		nullIfEmpty(job.ActorID),
		nullIfEmpty(job.RequestID),
		nullIfEmpty(job.CorrelationID),
		inputJSON,
		job.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("create job: %w", err)
	}
	return nil
}

func (s *Store) Job(ctx context.Context, id string) (JobRecord, error) {
	row := s.db.QueryRowContext(ctx, jobSelect+` WHERE id = ?`, id)
	job, err := scanJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return JobRecord{}, ErrJobNotFound
	}
	return job, err
}

func (s *Store) ListJobs(ctx context.Context, status string, limit int) ([]JobRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	query := jobSelect
	args := []any{}
	if status != "" {
		query += " WHERE status = ?"
		args = append(args, status)
	}
	query += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	defer rows.Close()

	jobs := make([]JobRecord, 0)
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate jobs: %w", err)
	}
	return jobs, nil
}

func (s *Store) ClaimNextJob(ctx context.Context, nodeID string, now time.Time) (JobRecord, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return JobRecord{}, fmt.Errorf("begin claim job: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var id string
	err = tx.QueryRowContext(ctx, `
		SELECT id FROM jobs
		WHERE node_id = ? AND status = 'queued'
		ORDER BY created_at ASC
		LIMIT 1
	`, nodeID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return JobRecord{}, ErrJobNotFound
	}
	if err != nil {
		return JobRecord{}, fmt.Errorf("select queued job: %w", err)
	}

	startedAt := now.UTC().Format(time.RFC3339Nano)
	result, err := tx.ExecContext(ctx, `
		UPDATE jobs
		SET status = 'running', started_at = ?, message = ''
		WHERE id = ? AND status = 'queued'
	`, startedAt, id)
	if err != nil {
		return JobRecord{}, fmt.Errorf("claim queued job: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		return JobRecord{}, ErrJobNotFound
	}
	if err := tx.Commit(); err != nil {
		return JobRecord{}, fmt.Errorf("commit job claim: %w", err)
	}
	return s.Job(ctx, id)
}

func (s *Store) UpdateJobProgress(ctx context.Context, id string, progress int, message string) error {
	if progress < 0 || progress > 10000 {
		return fmt.Errorf("progress must be between 0 and 10000")
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE jobs SET progress = ?, message = ?
		WHERE id = ? AND status IN ('running', 'cancel_requested')
	`, progress, message, id)
	if err != nil {
		return fmt.Errorf("update job progress: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrJobNotFound
	}
	return nil
}

func (s *Store) CompleteJob(ctx context.Context, id string, resultData map[string]any, now time.Time) error {
	resultJSON, err := marshalNullableObject(resultData)
	if err != nil {
		return fmt.Errorf("encode job result: %w", err)
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE jobs
		SET status = 'succeeded', progress = 10000, result_json = ?,
			completed_at = ?, message = '', error_code = NULL, error_message = NULL
		WHERE id = ? AND status IN ('running', 'cancel_requested')
	`, resultJSON, now.UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return fmt.Errorf("complete job: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrJobNotFound
	}
	return nil
}

func (s *Store) FailJob(ctx context.Context, id, code, message string, now time.Time) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE jobs
		SET status = 'failed', error_code = ?, error_message = ?,
			completed_at = ?, message = ?
		WHERE id = ? AND status IN ('running', 'cancel_requested')
	`, code, message, now.UTC().Format(time.RFC3339Nano), message, id)
	if err != nil {
		return fmt.Errorf("fail job: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrJobNotFound
	}
	return nil
}

func (s *Store) MarkJobCancelled(ctx context.Context, id, message string, now time.Time) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE jobs
		SET status = 'cancelled', completed_at = ?, message = ?
		WHERE id = ? AND status IN ('queued', 'running', 'cancel_requested')
	`, now.UTC().Format(time.RFC3339Nano), message, id)
	if err != nil {
		return fmt.Errorf("mark job cancelled: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrJobNotFound
	}
	return nil
}

func (s *Store) RequestJobCancel(ctx context.Context, id string, now time.Time) (JobRecord, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return JobRecord{}, fmt.Errorf("begin job cancellation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var status string
	if err := tx.QueryRowContext(ctx, "SELECT status FROM jobs WHERE id = ?", id).Scan(&status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return JobRecord{}, ErrJobNotFound
		}
		return JobRecord{}, fmt.Errorf("read job for cancellation: %w", err)
	}

	timestamp := now.UTC().Format(time.RFC3339Nano)
	switch status {
	case "queued":
		_, err = tx.ExecContext(ctx, `
			UPDATE jobs
			SET status = 'cancelled', cancel_requested_at = ?, completed_at = ?, message = 'cancelled before start'
			WHERE id = ?
		`, timestamp, timestamp, id)
	case "running":
		_, err = tx.ExecContext(ctx, `
			UPDATE jobs
			SET status = 'cancel_requested', cancel_requested_at = ?
			WHERE id = ?
		`, timestamp, id)
	case "cancel_requested":
		// Idempotent cancellation request.
	case "cancelled", "succeeded", "failed":
		return JobRecord{}, ErrJobTerminal
	default:
		return JobRecord{}, fmt.Errorf("unknown job status %q", status)
	}
	if err != nil {
		return JobRecord{}, fmt.Errorf("request job cancellation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return JobRecord{}, fmt.Errorf("commit job cancellation: %w", err)
	}
	return s.Job(ctx, id)
}

func (s *Store) ClearTerminalJobs(ctx context.Context) (int64, error) {
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM jobs
		WHERE status IN ('succeeded', 'failed', 'cancelled')
	`)
	if err != nil {
		return 0, fmt.Errorf("clear terminal jobs: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read cleared job count: %w", err)
	}
	return count, nil
}

func (s *Store) RecoverInterruptedJobs(ctx context.Context, nodeID string) (int64, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE jobs
		SET status = 'queued',
			started_at = NULL,
			cancel_requested_at = NULL,
			message = 'recovered after core restart'
		WHERE node_id = ? AND status IN ('running', 'cancel_requested')
	`, nodeID)
	if err != nil {
		return 0, fmt.Errorf("recover interrupted jobs: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read recovered job count: %w", err)
	}
	return count, nil
}

const jobSelect = `
	SELECT
		id, node_id, type, status, progress, message,
		actor_type, COALESCE(actor_id, ''),
		COALESCE(request_id, ''), COALESCE(correlation_id, ''),
		input_json, COALESCE(result_json, ''),
		COALESCE(error_code, ''), COALESCE(error_message, ''),
		created_at, started_at, completed_at, cancel_requested_at
	FROM jobs
`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanJob(scanner rowScanner) (JobRecord, error) {
	var job JobRecord
	var inputJSON, resultJSON, createdAt string
	var startedAt, completedAt, cancelRequestedAt sql.NullString

	err := scanner.Scan(
		&job.ID, &job.NodeID, &job.Type, &job.Status, &job.Progress, &job.Message,
		&job.ActorType, &job.ActorID, &job.RequestID, &job.CorrelationID,
		&inputJSON, &resultJSON, &job.ErrorCode, &job.ErrorMessage,
		&createdAt, &startedAt, &completedAt, &cancelRequestedAt,
	)
	if err != nil {
		return JobRecord{}, err
	}
	if err := json.Unmarshal([]byte(inputJSON), &job.Input); err != nil {
		return JobRecord{}, fmt.Errorf("decode job input: %w", err)
	}
	if resultJSON != "" {
		if err := json.Unmarshal([]byte(resultJSON), &job.Result); err != nil {
			return JobRecord{}, fmt.Errorf("decode job result: %w", err)
		}
	}
	if job.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return JobRecord{}, fmt.Errorf("parse job created_at: %w", err)
	}
	if job.StartedAt, err = parseOptionalTime(startedAt); err != nil {
		return JobRecord{}, err
	}
	if job.CompletedAt, err = parseOptionalTime(completedAt); err != nil {
		return JobRecord{}, err
	}
	if job.CancelRequestedAt, err = parseOptionalTime(cancelRequestedAt); err != nil {
		return JobRecord{}, err
	}
	return job, nil
}

func parseOptionalTime(value sql.NullString) (*time.Time, error) {
	if !value.Valid {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value.String)
	if err != nil {
		return nil, fmt.Errorf("parse optional timestamp: %w", err)
	}
	return &parsed, nil
}

func marshalObject(value map[string]any) (string, error) {
	if value == nil {
		value = map[string]any{}
	}
	raw, err := json.Marshal(value)
	return string(raw), err
}

func marshalNullableObject(value map[string]any) (any, error) {
	if value == nil {
		return nil, nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return string(raw), nil
}
