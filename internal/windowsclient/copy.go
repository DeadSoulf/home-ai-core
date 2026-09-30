package windowsclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Transfer is an immutable source snapshot for one item in a copy plan.
type Transfer struct {
	Kind        string `json:"kind"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	SizeBytes   int64  `json:"size_bytes"`
	ModTimeNS   int64  `json:"mod_time_ns"`
	SHA256      string `json:"sha256"`
}

// PlanCopy inspects the entire source before returning a deterministic,
// parent-first plan. No symlink or special file is followed or included.
func PlanCopy(source, destination string) ([]Transfer, error) {
	if strings.TrimSpace(source) == "" {
		return nil, errors.New("source path is required")
	}
	absolute, err := filepath.Abs(source)
	if err != nil {
		return nil, fmt.Errorf("resolve source path: %w", err)
	}
	info, err := inspectCopySource(absolute)
	if err != nil {
		return nil, err
	}
	defaultDestination := strings.TrimSpace(destination) == ""
	if defaultDestination {
		destination = filepath.Base(absolute)
	}
	normalized, err := normalizeCopyDestination(destination)
	if err != nil {
		return nil, err
	}
	if defaultDestination && normalized != destination {
		return nil, errors.New("source name cannot be preserved as a destination; choose an explicit destination")
	}
	destination = normalized
	if !info.IsDir() {
		transfer, err := snapshotCopyItem(absolute, destination, info)
		if err != nil {
			return nil, err
		}
		return []Transfer{transfer}, nil
	}

	var transfers []Transfer
	err = filepath.WalkDir(absolute, func(sourcePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("inspect copy tree: %w", walkErr)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("source symlinks are not supported: %s", sourcePath)
		}
		info, err := inspectCopySource(sourcePath)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(absolute, sourcePath)
		if err != nil {
			return err
		}
		target := destination
		if relative != "." {
			// Backslashes cannot be preserved as names in the portable API path.
			if filepath.Separator != '\\' && strings.Contains(relative, "\\") {
				return fmt.Errorf("source name contains a path separator: %s", sourcePath)
			}
			target = destination + "/" + filepath.ToSlash(relative)
			canonical, err := normalizeCopyDestination(target)
			if err != nil {
				return fmt.Errorf("invalid destination for %s: %w", sourcePath, err)
			}
			if canonical != target {
				return fmt.Errorf("source name cannot be preserved as a destination: %s", sourcePath)
			}
		}
		transfer, err := snapshotCopyItem(sourcePath, target, info)
		if err != nil {
			return err
		}
		transfers = append(transfers, transfer)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return transfers, nil
}

func snapshotCopyItem(source, destination string, info os.FileInfo) (Transfer, error) {
	transfer := Transfer{
		Source: source, Destination: destination, ModTimeNS: info.ModTime().UnixNano(),
	}
	if info.IsDir() {
		transfer.Kind = "directory"
		return transfer, nil
	}
	transfer.Kind = "file"
	transfer.SizeBytes = info.Size()
	checksum, err := hashCopySource(source, info)
	if err != nil {
		return Transfer{}, err
	}
	transfer.SHA256 = checksum
	return transfer, nil
}

// CopyTransfer safely executes or recovers one planned transfer. An existing
// destination is accepted only after proving it has the intended kind/content.
func (c *Client) CopyTransfer(ctx context.Context, folderID string, transfer Transfer, options UploadOptions) (UploadResult, error) {
	folderID = strings.TrimSpace(folderID)
	if folderID == "" {
		return UploadResult{}, errors.New("folder ID is required")
	}
	var err error
	transfer.Destination, err = normalizeCopyDestination(transfer.Destination)
	if err != nil {
		return UploadResult{}, err
	}
	if err := validateCopySource(transfer); err != nil {
		return UploadResult{}, err
	}
	result, exists, err := c.VerifyTransfer(ctx, folderID, transfer)
	if err != nil || exists {
		return result, err
	}
	parent := path.Dir(transfer.Destination)
	if transfer.Kind == "directory" {
		parent = transfer.Destination
	}
	if parent != "." {
		if err := c.ensureCopyDirectories(ctx, folderID, parent); err != nil {
			return UploadResult{}, err
		}
	}
	if transfer.Kind == "directory" {
		return UploadResult{Path: transfer.Destination}, nil
	}
	options.Destination = transfer.Destination
	options.copySnapshot = &transfer
	result, err = c.UploadFile(ctx, folderID, transfer.Source, options)
	if err == nil {
		if result.Path != transfer.Destination || result.SizeBytes != transfer.SizeBytes ||
			!strings.EqualFold(result.SHA256, transfer.SHA256) {
			return UploadResult{}, errors.New("completed upload does not match the copy snapshot")
		}
		return result, nil
	}
	// The server may have committed the upload before its response was lost.
	// A fresh content download proves completion without a second write.
	recovered, exists, verifyErr := c.VerifyTransfer(ctx, folderID, transfer)
	if verifyErr != nil {
		return UploadResult{}, errors.Join(err, fmt.Errorf("verify destination after upload failure: %w", verifyErr))
	}
	if exists {
		return recovered, nil
	}
	return UploadResult{}, err
}

// VerifyTransfer checks remote completion without requiring the source to still
// exist. A different existing item is an error, never a reason to overwrite it.
func (c *Client) VerifyTransfer(ctx context.Context, folderID string, transfer Transfer) (UploadResult, bool, error) {
	folderID = strings.TrimSpace(folderID)
	if folderID == "" {
		return UploadResult{}, false, errors.New("folder ID is required")
	}
	destination, err := normalizeCopyDestination(transfer.Destination)
	if err != nil {
		return UploadResult{}, false, err
	}
	if transfer.Kind != "directory" && transfer.Kind != "file" {
		return UploadResult{}, false, errors.New("copy transfer kind must be directory or file")
	}
	if transfer.Kind == "file" {
		if err := validateCopyChecksum(transfer); err != nil {
			return UploadResult{}, false, err
		}
	}
	entry, exists, err := c.findCopyEntry(ctx, folderID, destination)
	if err != nil || !exists {
		return UploadResult{}, false, err
	}
	if entry.Kind != transfer.Kind {
		return UploadResult{}, false, fmt.Errorf("destination %s already exists as %s, want %s", destination, entry.Kind, transfer.Kind)
	}
	result := UploadResult{Path: destination}
	if transfer.Kind == "directory" {
		return result, true, nil
	}
	if entry.SizeBytes != transfer.SizeBytes {
		return UploadResult{}, false, fmt.Errorf("destination %s already exists with different content", destination)
	}
	endpoint := "/api/v1/files/folders/" + url.PathEscape(folderID) + "/content?path=" + url.QueryEscape(destination)
	// Readback may take longer than a chunk request. Preserve transport and
	// other client settings, but bound inactivity instead of total duration.
	streamClient := *c.HTTPClient
	idleTimeout := streamClient.Timeout
	if idleTimeout <= 0 {
		idleTimeout = 90 * time.Second
	}
	streamClient.Timeout = 0
	streamCtx, activity := newCopyStreamActivity(ctx, idleTimeout)
	defer activity.stop()
	request, err := c.newRequest(streamCtx, http.MethodGet, endpoint, nil, true)
	if err != nil {
		return UploadResult{}, false, err
	}
	request.Header.Set("Accept", "application/octet-stream")
	response, err := streamClient.Do(request)
	if err != nil {
		if cause := context.Cause(streamCtx); cause != nil {
			err = cause
		}
		return UploadResult{}, false, err
	}
	activity.progress()
	response.Body = &copyStreamBody{ReadCloser: response.Body, ctx: streamCtx, activity: activity}
	// Closing the body on cancellation also supports custom transports whose
	// body readers unblock on Close rather than request-context cancellation.
	stopClose := context.AfterFunc(streamCtx, func() { _ = response.Body.Close() })
	defer stopClose()
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		err := decodeHTTPError(response)
		if cause := context.Cause(streamCtx); cause != nil {
			err = cause
		}
		return UploadResult{}, false, err
	}
	hasher := sha256.New()
	size, err := io.Copy(hasher, io.LimitReader(response.Body, transfer.SizeBytes+1))
	if err != nil {
		return UploadResult{}, false, fmt.Errorf("read destination for verification: %w", err)
	}
	checksum := hex.EncodeToString(hasher.Sum(nil))
	if size != transfer.SizeBytes || !strings.EqualFold(checksum, transfer.SHA256) {
		return UploadResult{}, false, fmt.Errorf("destination %s already exists with different content", destination)
	}
	result.SizeBytes, result.SHA256 = size, checksum
	return result, true, nil
}

type copyStreamActivity struct {
	mu       sync.Mutex
	timer    *time.Timer
	deadline time.Time
	timeout  time.Duration
	cancel   context.CancelCauseFunc
	stopped  bool
}

func newCopyStreamActivity(parent context.Context, timeout time.Duration) (context.Context, *copyStreamActivity) {
	ctx, cancel := context.WithCancelCause(parent)
	activity := &copyStreamActivity{timeout: timeout, deadline: time.Now().Add(timeout), cancel: cancel}
	activity.mu.Lock()
	activity.timer = time.AfterFunc(timeout, activity.expire)
	activity.mu.Unlock()
	return ctx, activity
}

func (activity *copyStreamActivity) expire() {
	activity.mu.Lock()
	defer activity.mu.Unlock()
	if activity.stopped {
		return
	}
	// A timer callback already queued before new bytes arrived may still run.
	// Check the current deadline so that old callbacks cannot cancel progress.
	if remaining := time.Until(activity.deadline); remaining > 0 {
		activity.timer.Reset(remaining)
		return
	}
	activity.stopped = true
	activity.cancel(fmt.Errorf("destination verification inactive for %s: %w", activity.timeout, context.DeadlineExceeded))
}

func (activity *copyStreamActivity) progress() {
	activity.mu.Lock()
	defer activity.mu.Unlock()
	if !activity.stopped {
		activity.deadline = time.Now().Add(activity.timeout)
		activity.timer.Reset(activity.timeout)
	}
}

func (activity *copyStreamActivity) stop() {
	activity.mu.Lock()
	defer activity.mu.Unlock()
	activity.stopped = true
	activity.timer.Stop()
	activity.cancel(nil)
}

type copyStreamBody struct {
	io.ReadCloser
	ctx       context.Context
	activity  *copyStreamActivity
	closeOnce sync.Once
	closeErr  error
}

func (body *copyStreamBody) Close() error {
	body.closeOnce.Do(func() { body.closeErr = body.ReadCloser.Close() })
	return body.closeErr
}

func (body *copyStreamBody) Read(data []byte) (int, error) {
	if cause := context.Cause(body.ctx); cause != nil {
		return 0, cause
	}
	n, err := body.ReadCloser.Read(data)
	if n > 0 {
		body.activity.progress()
	}
	if cause := context.Cause(body.ctx); cause != nil {
		return n, cause
	}
	return n, err
}

type copyEntry struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	SizeBytes int64  `json:"size_bytes"`
}

func (c *Client) findCopyEntry(ctx context.Context, folderID, destination string) (copyEntry, bool, error) {
	parent := ""
	segments := strings.Split(destination, "/")
	for i, segment := range segments {
		var response struct {
			Entries []copyEntry `json:"entries"`
		}
		endpoint := "/api/v1/files/folders/" + url.PathEscape(folderID) + "/entries?path=" + url.QueryEscape(parent)
		if err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response, true); err != nil {
			return copyEntry{}, false, err
		}
		candidate := segment
		if parent != "" {
			candidate = parent + "/" + segment
		}
		var found *copyEntry
		for j := range response.Entries {
			if response.Entries[j].Name == segment {
				found = &response.Entries[j]
				break
			}
		}
		if found == nil {
			return copyEntry{}, false, nil
		}
		if found.Path != candidate {
			return copyEntry{}, false, errors.New("server returned an unexpected destination path")
		}
		if i == len(segments)-1 {
			return *found, true, nil
		}
		if found.Kind != "directory" {
			return copyEntry{}, false, fmt.Errorf("destination ancestor %s is not a directory", candidate)
		}
		parent = candidate
	}
	return copyEntry{}, false, nil
}

func (c *Client) ensureCopyDirectories(ctx context.Context, folderID, destination string) error {
	current := ""
	for _, segment := range strings.Split(destination, "/") {
		if current == "" {
			current = segment
		} else {
			current += "/" + segment
		}
		transfer := Transfer{Kind: "directory", Destination: current}
		if _, exists, err := c.VerifyTransfer(ctx, folderID, transfer); err != nil {
			return err
		} else if exists {
			continue
		}
		endpoint := "/api/v1/files/folders/" + url.PathEscape(folderID) + "/directories"
		err := c.doJSON(ctx, http.MethodPost, endpoint, map[string]string{"path": current}, nil, true)
		// Creation can race with another copy, or its successful response can be
		// lost. In either case accept only a freshly verified real directory.
		if err != nil {
			if _, exists, verifyErr := c.VerifyTransfer(ctx, folderID, transfer); verifyErr == nil && exists {
				continue
			} else if verifyErr != nil {
				return errors.Join(err, verifyErr)
			}
			return err
		}
	}
	return nil
}

func normalizeCopyDestination(destination string) (string, error) {
	destination = strings.ReplaceAll(strings.TrimSpace(destination), "\\", "/")
	if destination == "" || strings.HasPrefix(destination, "/") || strings.Contains(destination, ":") {
		return "", errors.New("destination must be a safe relative path")
	}
	for _, r := range destination {
		if r < 32 || r == 127 {
			return "", errors.New("destination contains a control character")
		}
	}
	for _, segment := range strings.Split(destination, "/") {
		if segment == ".." {
			return "", errors.New("destination cannot contain parent traversal")
		}
		if segment == ".home-ai-trash" || strings.HasPrefix(segment, ".home-ai-upload-") {
			return "", errors.New("destination uses a reserved server path")
		}
	}
	destination = path.Clean(destination)
	if destination == "." {
		return "", errors.New("destination must name a file or directory")
	}
	return destination, nil
}

func inspectCopySource(source string) (os.FileInfo, error) {
	if !filepath.IsAbs(source) {
		return nil, errors.New("copy source must be absolute")
	}
	var components []string
	for current := filepath.Clean(source); ; current = filepath.Dir(current) {
		components = append(components, current)
		if filepath.Dir(current) == current {
			break
		}
	}
	var result os.FileInfo
	for i := len(components) - 1; i >= 0; i-- {
		info, err := os.Lstat(components[i])
		if err != nil {
			return nil, fmt.Errorf("inspect copy source: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("source symlinks are not supported: %s", components[i])
		}
		if i > 0 && !info.IsDir() {
			return nil, fmt.Errorf("source ancestor is not a directory: %s", components[i])
		}
		result = info
	}
	if !result.IsDir() && !result.Mode().IsRegular() {
		return nil, errors.New("copy source must be a regular file or directory")
	}
	return result, nil
}

func validateCopyChecksum(transfer Transfer) error {
	checksum, err := hex.DecodeString(transfer.SHA256)
	if err != nil || len(checksum) != sha256.Size || transfer.SizeBytes < 0 || transfer.SizeBytes == int64(^uint64(0)>>1) {
		return errors.New("file copy snapshot has invalid size or SHA-256")
	}
	return nil
}

func validateCopySource(transfer Transfer) error {
	info, err := inspectCopySource(transfer.Source)
	if err != nil {
		return err
	}
	if transfer.Kind == "directory" {
		if !info.IsDir() {
			return errors.New("queued source directory changed its kind")
		}
		return nil
	}
	if transfer.Kind != "file" {
		return errors.New("copy transfer kind must be directory or file")
	}
	if err := validateCopyChecksum(transfer); err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() != transfer.SizeBytes || info.ModTime().UnixNano() != transfer.ModTimeNS {
		return errors.New("queued source file changed since it was planned")
	}
	checksum, err := hashCopySource(transfer.Source, info)
	if err != nil {
		return err
	}
	if !strings.EqualFold(checksum, transfer.SHA256) {
		return errors.New("queued source file changed since it was planned")
	}
	return nil
}

func hashCopySource(source string, before os.FileInfo) (string, error) {
	file, err := os.Open(source)
	if err != nil {
		return "", fmt.Errorf("open copy source: %w", err)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !opened.Mode().IsRegular() || !sameCopySnapshot(before, opened) {
		return "", errors.New("source file changed while planning or validating the copy")
	}
	hasher := sha256.New()
	size, err := io.Copy(hasher, file)
	if err != nil {
		return "", fmt.Errorf("hash copy source: %w", err)
	}
	after, err := inspectCopySource(source)
	if err != nil {
		return "", err
	}
	if size != before.Size() || !sameCopySnapshot(before, after) {
		return "", errors.New("source file changed while planning or validating the copy")
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func sameCopySnapshot(before, after os.FileInfo) bool {
	return os.SameFile(before, after) && before.Size() == after.Size() && before.ModTime().Equal(after.ModTime())
}
