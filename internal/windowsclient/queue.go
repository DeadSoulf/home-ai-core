package windowsclient

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	queueVersion  = 1
	maxQueueBytes = 64 << 20
)

const (
	QueuePending   = "pending"
	QueueRunning   = "running"
	QueueCompleted = "completed"
	QueueFailed    = "failed"
)

// QueueProfile identifies the login to use. Authentication secrets are never
// part of the on-disk queue.
type QueueProfile struct {
	ServerURL string `json:"server_url"`
	Username  string `json:"username"`
}

type QueueTransfer struct {
	Transfer
	ID            string `json:"id"`
	Status        string `json:"status"`
	Error         string `json:"error,omitempty"`
	UploadedBytes int64  `json:"uploaded_bytes"`
}

type QueueJob struct {
	ID           string          `json:"id"`
	FolderID     string          `json:"folder_id"`
	Source       string          `json:"source"`
	Destination  string          `json:"destination"`
	RestartStale bool            `json:"restart_stale"`
	Status       string          `json:"status"`
	Error        string          `json:"error,omitempty"`
	Transfers    []QueueTransfer `json:"transfers"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

type QueueProgress struct {
	JobID              string
	TransferID         string
	Path               string
	UploadedBytes      int64
	TotalBytes         int64
	Resumed            bool
	Status             string
	CompletedTransfers int
	TotalTransfers     int
}

type queueState struct {
	Version int          `json:"version"`
	Profile QueueProfile `json:"profile"`
	Jobs    []QueueJob   `json:"jobs"`
}

// Queue keeps an operating-system lock for its entire lifetime. Always Close
// it after an operation; locks are also released automatically on process exit.
type Queue struct {
	opMu     sync.Mutex
	mu       sync.Mutex
	filename string
	lock     *os.File
	state    queueState
	closed   bool
}

func OpenQueue(filename string) (*Queue, error) {
	if strings.TrimSpace(filename) == "" {
		return nil, errors.New("queue path is required")
	}
	filename, err := filepath.Abs(filename)
	if err != nil {
		return nil, fmt.Errorf("resolve queue path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
		return nil, fmt.Errorf("create queue directory: %w", err)
	}
	// Resolve directory aliases so two paths to the same queue share a lock.
	dir, err := filepath.EvalSymlinks(filepath.Dir(filename))
	if err != nil {
		return nil, fmt.Errorf("resolve queue directory: %w", err)
	}
	filename = filepath.Join(dir, filepath.Base(filename))
	for _, name := range []string{filename, filename + ".lock"} {
		if info, err := os.Lstat(name); err == nil {
			if !info.Mode().IsRegular() {
				return nil, fmt.Errorf("queue path must be a regular file: %s", name)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("inspect queue path: %w", err)
		}
	}
	lock, err := lockQueueFile(filename + ".lock")
	if err != nil {
		return nil, err
	}
	q := &Queue{filename: filename, lock: lock, state: queueState{Version: queueVersion, Jobs: []QueueJob{}}}
	cleanup := func(err error) (*Queue, error) {
		_ = unlockQueueFile(lock)
		_ = lock.Close()
		return nil, err
	}
	file, err := os.Open(filename)
	if errors.Is(err, os.ErrNotExist) {
		if err := q.persist(q.state); err != nil {
			return cleanup(err)
		}
		return q, nil
	}
	if err != nil {
		return cleanup(fmt.Errorf("open queue: %w", err))
	}
	info, err := file.Stat()
	if err != nil || info.Size() > maxQueueBytes {
		_ = file.Close()
		if err == nil {
			err = errors.New("queue exceeds 64 MiB")
		}
		return cleanup(fmt.Errorf("inspect queue: %w", err))
	}
	decoder := json.NewDecoder(io.LimitReader(file, maxQueueBytes+1))
	decoder.DisallowUnknownFields()
	q.state = queueState{}
	decodeErr := decoder.Decode(&q.state)
	if decodeErr == nil {
		var extra any
		if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
			decodeErr = errors.New("queue must contain exactly one JSON object")
		}
	}
	closeErr := file.Close()
	if decodeErr != nil {
		return cleanup(fmt.Errorf("decode queue: %w", decodeErr))
	}
	if closeErr != nil {
		return cleanup(fmt.Errorf("close queue: %w", closeErr))
	}
	if err := validateQueue(q.state); err != nil {
		return cleanup(fmt.Errorf("invalid queue: %w", err))
	}
	recovered := false
	for i := range q.state.Jobs {
		job := &q.state.Jobs[i]
		for j := range job.Transfers {
			if job.Transfers[j].Status == QueueRunning {
				job.Transfers[j].Status = QueuePending
				recovered = true
			}
		}
		if job.Status == QueueRunning {
			refreshQueueJob(job)
			job.UpdatedAt = time.Now().UTC()
		}
	}
	if recovered {
		if err := q.persist(q.state); err != nil {
			return cleanup(err)
		}
	}
	return q, nil
}

func (q *Queue) Close() error {
	q.opMu.Lock()
	defer q.opMu.Unlock()
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return nil
	}
	q.closed = true
	return errors.Join(unlockQueueFile(q.lock), q.lock.Close())
}

func (q *Queue) Profile() QueueProfile {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.state.Profile
}

// Jobs returns a detached snapshot, safe to inspect while Run reports progress.
func (q *Queue) Jobs() []QueueJob {
	q.mu.Lock()
	defer q.mu.Unlock()
	return cloneQueueState(q.state).Jobs
}

func (q *Queue) Add(server, username, folder, source, destination string, restartStale bool) (QueueJob, error) {
	q.opMu.Lock()
	defer q.opMu.Unlock()
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return QueueJob{}, errors.New("queue is closed")
	}
	server, err := normalizeQueueServer(server)
	if err != nil {
		return QueueJob{}, err
	}
	username = strings.TrimSpace(username)
	folder = strings.TrimSpace(folder)
	if username == "" || folder == "" {
		return QueueJob{}, errors.New("username and folder ID are required")
	}
	if strings.TrimSpace(source) == "" {
		return QueueJob{}, errors.New("source path is required")
	}
	profile := QueueProfile{ServerURL: server, Username: username}
	if q.state.Profile != (QueueProfile{}) && q.state.Profile != profile {
		return QueueJob{}, errors.New("queue belongs to a different server or username; use a separate queue file")
	}
	source, err = filepath.Abs(source)
	if err != nil {
		return QueueJob{}, fmt.Errorf("resolve source: %w", err)
	}
	transfers, err := PlanCopy(source, destination)
	if err != nil {
		return QueueJob{}, err
	}
	if len(transfers) == 0 {
		return QueueJob{}, errors.New("copy plan is empty")
	}
	id, err := queueID("job")
	if err != nil {
		return QueueJob{}, err
	}
	now := time.Now().UTC()
	job := QueueJob{ID: id, FolderID: folder, Source: source, Destination: transfers[0].Destination,
		RestartStale: restartStale, Status: QueuePending, CreatedAt: now, UpdatedAt: now,
		Transfers: make([]QueueTransfer, 0, len(transfers))}
	for _, transfer := range transfers {
		tid, err := queueID("transfer")
		if err != nil {
			return QueueJob{}, err
		}
		job.Transfers = append(job.Transfers, QueueTransfer{Transfer: transfer, ID: tid, Status: QueuePending})
	}
	next := cloneQueueState(q.state)
	next.Profile = profile
	next.Jobs = append(next.Jobs, job)
	if err := q.commit(next); err != nil {
		return QueueJob{}, err
	}
	return cloneQueueState(queueState{Jobs: []QueueJob{job}}).Jobs[0], nil
}

// Retry only resets failures. Completed entries retain their receipts and are
// never recopied, and the original transfer plan is never rebuilt.
func (q *Queue) Retry(jobID string) error {
	q.opMu.Lock()
	defer q.opMu.Unlock()
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return errors.New("queue is closed")
	}
	next := cloneQueueState(q.state)
	for i := range next.Jobs {
		job := &next.Jobs[i]
		if job.ID != jobID {
			continue
		}
		if job.Status != QueueFailed {
			return fmt.Errorf("queue job %s is not failed", jobID)
		}
		for j := range job.Transfers {
			if job.Transfers[j].Status == QueueFailed {
				job.Transfers[j].Status = QueuePending
				job.Transfers[j].Error = ""
			}
		}
		refreshQueueJob(job)
		job.UpdatedAt = time.Now().UTC()
		return q.commit(next)
	}
	return fmt.Errorf("queue job %s not found", jobID)
}

func (q *Queue) Run(ctx context.Context, client *Client, options UploadOptions, callback func(QueueProgress)) error {
	if client == nil || client.BaseURL == nil {
		return errors.New("queue requires a client with a server URL")
	}
	return q.run(ctx, client.BaseURL.String(), client.CopyTransfer, options, callback, client.Token)
}

type queueCopyFunc func(context.Context, string, Transfer, UploadOptions) (UploadResult, error)

func (q *Queue) run(ctx context.Context, server string, copyTransfer queueCopyFunc, options UploadOptions, callback func(QueueProgress), secret string) error {
	q.opMu.Lock()
	defer q.opMu.Unlock()
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return errors.New("queue is closed")
	}
	server, err := normalizeQueueServer(server)
	if err != nil || server != q.state.Profile.ServerURL {
		q.mu.Unlock()
		return errors.New("client server does not match the queue profile")
	}
	jobs := cloneQueueState(q.state).Jobs
	q.mu.Unlock()
	for i := range jobs {
		if err := ctx.Err(); err != nil {
			return err
		}
		job := jobs[i]
		if job.Status == QueueFailed {
			return fmt.Errorf("queue job %s failed: %s; retry this job explicitly before continuing", job.ID, job.Error)
		}
		for j := range job.Transfers {
			if err := ctx.Err(); err != nil {
				return err
			}
			transfer := job.Transfers[j]
			if transfer.Status == QueueCompleted {
				continue
			}
			if err := q.setTransfer(i, j, QueueRunning, "", transfer.UploadedBytes); err != nil {
				return err
			}
			q.emitProgress(i, j, false, callback)
			copyOptions := options
			copyOptions.RestartStale = job.RestartStale || options.RestartStale
			copyOptions.Destination = transfer.Destination
			copyOptions.Progress = func(progress Progress) {
				q.mu.Lock()
				amount := progress.UploadedBytes
				if amount < 0 {
					amount = 0
				}
				if amount > transfer.SizeBytes {
					amount = transfer.SizeBytes
				}
				q.state.Jobs[i].Transfers[j].UploadedBytes = amount
				q.mu.Unlock()
				if options.Progress != nil {
					options.Progress(progress)
				}
				q.emitProgress(i, j, progress.Resumed, callback)
			}
			_, copyErr := copyTransfer(ctx, job.FolderID, transfer.Transfer, copyOptions)
			status, message, uploaded := QueueCompleted, "", transfer.SizeBytes
			if copyErr != nil {
				q.mu.Lock()
				uploaded = q.state.Jobs[i].Transfers[j].UploadedBytes
				q.mu.Unlock()
				if ctx.Err() != nil || errors.Is(copyErr, context.Canceled) || errors.Is(copyErr, context.DeadlineExceeded) {
					status = QueuePending
				} else {
					status = QueueFailed
					message = queueError(copyErr, secret)
				}
			}
			if err := q.setTransfer(i, j, status, message, uploaded); err != nil {
				return fmt.Errorf("save queue after copying %s: %w", transfer.Destination, err)
			}
			q.emitProgress(i, j, false, callback)
			if copyErr != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return fmt.Errorf("queue job %s, %s: %w", job.ID, transfer.Destination, copyErr)
			}
		}
	}
	return nil
}

func (q *Queue) setTransfer(jobIndex, transferIndex int, status, message string, uploaded int64) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	next := cloneQueueState(q.state)
	job := &next.Jobs[jobIndex]
	transfer := &job.Transfers[transferIndex]
	transfer.Status, transfer.Error, transfer.UploadedBytes = status, message, uploaded
	refreshQueueJob(job)
	job.UpdatedAt = time.Now().UTC()
	return q.commit(next)
}

func (q *Queue) emitProgress(i, j int, resumed bool, callback func(QueueProgress)) {
	if callback == nil {
		return
	}
	q.mu.Lock()
	job := q.state.Jobs[i]
	transfer := job.Transfers[j]
	progress := QueueProgress{JobID: job.ID, TransferID: transfer.ID, Path: transfer.Destination,
		UploadedBytes: transfer.UploadedBytes, TotalBytes: transfer.SizeBytes, Resumed: resumed,
		Status: transfer.Status, TotalTransfers: len(job.Transfers)}
	for _, item := range job.Transfers {
		if item.Status == QueueCompleted {
			progress.CompletedTransfers++
		}
	}
	q.mu.Unlock()
	callback(progress)
}

func (q *Queue) commit(next queueState) error {
	if err := validateQueue(next); err != nil {
		return fmt.Errorf("invalid queue update: %w", err)
	}
	if err := q.persist(next); err != nil {
		return err
	}
	q.state = next
	return nil
}

func (q *Queue) persist(state queueState) error {
	return q.persistWithLimit(state, maxQueueBytes)
}

func (q *Queue) persistWithLimit(state queueState, limit int) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode queue: %w", err)
	}
	data = append(data, '\n')
	if len(data) > limit {
		return errors.New("queue exceeds its size limit; use a separate queue file for more jobs")
	}
	file, err := os.CreateTemp(filepath.Dir(q.filename), ".home-ai-queue-*")
	if err != nil {
		return fmt.Errorf("create queue checkpoint: %w", err)
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write queue checkpoint: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync queue checkpoint: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close queue checkpoint: %w", err)
	}
	if err := replaceQueueFile(name, q.filename); err != nil {
		return fmt.Errorf("replace queue checkpoint: %w", err)
	}
	return nil
}

func cloneQueueState(state queueState) queueState {
	copy := state
	copy.Jobs = append([]QueueJob{}, state.Jobs...)
	for i := range copy.Jobs {
		copy.Jobs[i].Transfers = append([]QueueTransfer{}, state.Jobs[i].Transfers...)
	}
	return copy
}

func refreshQueueJob(job *QueueJob) {
	job.Status, job.Error = QueueCompleted, ""
	for _, transfer := range job.Transfers {
		if transfer.Status == QueueFailed {
			job.Status, job.Error = QueueFailed, transfer.Error
			return
		}
		if transfer.Status == QueueRunning {
			job.Status = QueueRunning
		}
		if transfer.Status == QueuePending && job.Status != QueueRunning {
			job.Status = QueuePending
		}
	}
}

func queueID(prefix string) (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("create queue ID: %w", err)
	}
	return prefix + "-" + hex.EncodeToString(bytes[:]), nil
}

func queueError(err error, secret string) string {
	message := err.Error()
	if secret != "" {
		message = strings.ReplaceAll(message, secret, "[redacted]")
	}
	if len(message) > 4096 {
		message = message[:4096]
	}
	return message
}

func normalizeQueueServer(raw string) (string, error) {
	client, err := New(raw)
	if err != nil {
		return "", err
	}
	u := *client.BaseURL
	if u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" {
		return "", errors.New("queue server URL must not contain credentials, query parameters, or a fragment")
	}
	u.Scheme = strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if (u.Scheme == "https" && port == "443") || (u.Scheme == "http" && port == "80") {
		port = ""
	}
	if port != "" {
		u.Host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		u.Host = "[" + host + "]"
	} else {
		u.Host = host
	}
	u.Path = strings.TrimRight(u.Path, "/")
	return u.String(), nil
}

func validateQueue(state queueState) error {
	if state.Version != queueVersion {
		return fmt.Errorf("unsupported queue version %d", state.Version)
	}
	if state.Jobs == nil {
		return errors.New("jobs must be an array")
	}
	if state.Profile != (QueueProfile{}) || len(state.Jobs) > 0 {
		server, err := normalizeQueueServer(state.Profile.ServerURL)
		if err != nil || server != state.Profile.ServerURL || strings.TrimSpace(state.Profile.Username) == "" || strings.TrimSpace(state.Profile.Username) != state.Profile.Username {
			return errors.New("invalid queue profile")
		}
	}
	ids := map[string]bool{}
	for _, job := range state.Jobs {
		if job.ID == "" || ids[job.ID] || strings.TrimSpace(job.FolderID) == "" || strings.TrimSpace(job.FolderID) != job.FolderID || !filepath.IsAbs(job.Source) || len(job.Transfers) == 0 || job.CreatedAt.IsZero() || job.UpdatedAt.IsZero() || job.UpdatedAt.Before(job.CreatedAt) {
			return errors.New("invalid queue job identity, source, timestamps, or transfers")
		}
		ids[job.ID] = true
		if job.Transfers[0].Source != job.Source || job.Transfers[0].Destination != job.Destination || (job.Transfers[0].Kind == "file" && len(job.Transfers) != 1) {
			return errors.New("job source or destination does not match its plan")
		}
		destinations := map[string]bool{}
		directories := map[string]bool{}
		for _, transfer := range job.Transfers {
			if transfer.ID == "" || ids[transfer.ID] || !filepath.IsAbs(transfer.Source) || !validQueueDestination(transfer.Destination) || destinations[transfer.Destination] {
				return errors.New("invalid or duplicate queue transfer")
			}
			ids[transfer.ID], destinations[transfer.Destination] = true, true
			relative, err := filepath.Rel(job.Source, transfer.Source)
			if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				return errors.New("transfer source escapes its job source")
			}
			expectedDestination := job.Destination
			if relative != "." {
				if !directories[filepath.Dir(transfer.Source)] {
					return errors.New("copy plan must list source directories before their children")
				}
				expectedDestination += "/" + filepath.ToSlash(relative)
			}
			if transfer.Destination != expectedDestination {
				return errors.New("transfer destination does not match its source in the plan")
			}
			if transfer.Kind != "file" && transfer.Kind != "directory" {
				return errors.New("invalid transfer kind")
			}
			if transfer.SizeBytes < 0 || transfer.UploadedBytes < 0 || transfer.UploadedBytes > transfer.SizeBytes {
				return errors.New("invalid transfer size or progress")
			}
			if transfer.Kind == "directory" {
				directories[transfer.Source] = true
				if transfer.SizeBytes != 0 || transfer.SHA256 != "" {
					return errors.New("invalid directory transfer")
				}
			} else {
				checksum, err := hex.DecodeString(transfer.SHA256)
				if err != nil || len(checksum) != 32 {
					return errors.New("invalid file checksum")
				}
			}
			switch transfer.Status {
			case QueuePending, QueueRunning, QueueCompleted:
				if transfer.Error != "" {
					return errors.New("only failed transfers may have an error")
				}
			case QueueFailed:
				if transfer.Error == "" {
					return errors.New("failed transfer must have an error")
				}
			default:
				return errors.New("invalid transfer status")
			}
			if transfer.Status == QueueCompleted && transfer.UploadedBytes != transfer.SizeBytes {
				return errors.New("completed transfer has incomplete progress")
			}
		}
		if job.Destination != job.Transfers[0].Destination {
			return errors.New("job destination does not match its plan")
		}
		expected := job
		refreshQueueJob(&expected)
		if expected.Status != job.Status || expected.Error != job.Error {
			return errors.New("job status does not match its transfers")
		}
	}
	return nil
}

func validQueueDestination(destination string) bool {
	clean, err := normalizeCopyDestination(destination)
	return err == nil && clean == destination
}
