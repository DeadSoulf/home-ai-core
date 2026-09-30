package windowsclient

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultChunkSize = int64(8 << 20)
	defaultRetries   = 3
)

type Client struct {
	BaseURL    *url.URL
	HTTPClient *http.Client
	Token      string
}

type Folder struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	PoolName    string `json:"pool_name"`
	CanRead     bool   `json:"can_read"`
	CanWrite    bool   `json:"can_write"`
}

type UploadChunk struct {
	Offset int64  `json:"offset"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type UploadSession struct {
	ID                string        `json:"id"`
	Path              string        `json:"path"`
	TotalBytes        int64         `json:"total_bytes"`
	ReceivedBytes     int64         `json:"received_bytes"`
	ExpectedSHA256    string        `json:"expected_sha256,omitempty"`
	ClientFingerprint string        `json:"client_fingerprint,omitempty"`
	Chunks            []UploadChunk `json:"chunks,omitempty"`
}

type UploadResult struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
}

type UploadOptions struct {
	Destination string
	ChunkSize   int64
	Retries     int
	RestartStale bool
	Progress    func(Progress)
}

type Progress struct {
	Path          string
	UploadedBytes int64
	TotalBytes    int64
	Resumed       bool
}

type apiErrorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type HTTPError struct {
	Status int
	Code   string
	Message string
}

func (e *HTTPError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("Home-AI API returned HTTP %d", e.Status)
}

func New(rawBaseURL string) (*Client, error) {
	rawBaseURL = strings.TrimSpace(rawBaseURL)
	if rawBaseURL == "" {
		return nil, errors.New("server URL is required")
	}
	parsed, err := url.Parse(rawBaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse server URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("server URL must use http or https")
	}
	if parsed.Host == "" {
		return nil, errors.New("server URL must include a host")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return &Client{
		BaseURL: parsed,
		HTTPClient: &http.Client{Timeout: 90 * time.Second},
	}, nil
}

func (c *Client) Login(ctx context.Context, username, password string) error {
	payload := map[string]string{
		"username": strings.TrimSpace(username),
		"password": password,
		"session_mode": "token",
	}
	var response struct {
		Token string `json:"token"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/auth/login", payload, &response, false); err != nil {
		return err
	}
	if strings.TrimSpace(response.Token) == "" {
		return errors.New("Home-AI login returned no token")
	}
	c.Token = response.Token
	return nil
}

func (c *Client) Folders(ctx context.Context) ([]Folder, error) {
	var response struct {
		Folders []Folder `json:"folders"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/files/folders", nil, &response, true); err != nil {
		return nil, err
	}
	return response.Folders, nil
}

func (c *Client) UploadFile(
	ctx context.Context,
	folderID string,
	sourcePath string,
	options UploadOptions,
) (UploadResult, error) {
	folderID = strings.TrimSpace(folderID)
	if folderID == "" {
		return UploadResult{}, errors.New("folder ID is required")
	}
	info, err := os.Stat(sourcePath)
	if err != nil {
		return UploadResult{}, fmt.Errorf("inspect source file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return UploadResult{}, errors.New("source must be a regular file")
	}

	destination := strings.TrimSpace(options.Destination)
	if destination == "" {
		destination = filepath.Base(sourcePath)
	}
	destination = strings.ReplaceAll(destination, "\\", "/")
	if destination == "" || strings.HasPrefix(destination, "/") || destination == ".." ||
		strings.HasPrefix(destination, "../") {
		return UploadResult{}, errors.New("destination must be a relative path")
	}

	chunkSize := options.ChunkSize
	if chunkSize <= 0 || chunkSize > DefaultChunkSize {
		chunkSize = DefaultChunkSize
	}
	retries := options.Retries
	if retries <= 0 {
		retries = defaultRetries
	}

	fullSHA, err := hashFile(sourcePath)
	if err != nil {
		return UploadResult{}, err
	}
	fingerprint := fileFingerprint(info)

	uploads, err := c.listUploads(ctx, folderID)
	if err != nil {
		return UploadResult{}, err
	}
	var session *UploadSession
	for i := range uploads {
		current := &uploads[i]
		if current.Path != destination {
			continue
		}
		if current.TotalBytes == info.Size() && current.ClientFingerprint == fingerprint {
			session = current
			break
		}
		if !options.RestartStale {
			return UploadResult{}, fmt.Errorf(
				"unfinished upload already exists for %s; rerun with stale upload replacement enabled",
				destination,
			)
		}
		if err := c.cancelUpload(ctx, folderID, current.ID); err != nil {
			return UploadResult{}, fmt.Errorf("cancel stale upload: %w", err)
		}
	}

	resumed := false
	if session != nil {
		if session.ExpectedSHA256 != "" && session.ExpectedSHA256 != fullSHA {
			if !options.RestartStale {
				return UploadResult{}, errors.New("existing upload checksum does not match selected file")
			}
			if err := c.cancelUpload(ctx, folderID, session.ID); err != nil {
				return UploadResult{}, err
			}
			session = nil
		} else {
			ok, err := verifyUploadedChunks(sourcePath, session.Chunks)
			if err != nil {
				return UploadResult{}, err
			}
			if !ok {
				if !options.RestartStale {
					return UploadResult{}, errors.New("existing upload chunks do not match selected file")
				}
				if err := c.cancelUpload(ctx, folderID, session.ID); err != nil {
					return UploadResult{}, err
				}
				session = nil
			} else {
				resumed = session.ReceivedBytes > 0
			}
		}
	}

	if session == nil {
		created, err := c.createUpload(ctx, folderID, destination, info.Size(), fullSHA, fingerprint)
		if err != nil {
			return UploadResult{}, err
		}
		session = &created
	}

	file, err := os.Open(sourcePath)
	if err != nil {
		return UploadResult{}, fmt.Errorf("open source file: %w", err)
	}
	defer file.Close()

	offset := session.ReceivedBytes
	if offset < 0 || offset > info.Size() {
		return UploadResult{}, errors.New("server returned invalid upload offset")
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return UploadResult{}, fmt.Errorf("seek source file: %w", err)
	}
	reportProgress(options.Progress, destination, offset, info.Size(), resumed)

	buffer := make([]byte, int(chunkSize))
	for offset < info.Size() {
		remaining := info.Size() - offset
		size := int64(len(buffer))
		if remaining < size {
			size = remaining
		}
		n, readErr := io.ReadFull(file, buffer[:size])
		if readErr != nil && !errors.Is(readErr, io.EOF) && !errors.Is(readErr, io.ErrUnexpectedEOF) {
			return UploadResult{}, fmt.Errorf("read source chunk: %w", readErr)
		}
		if n == 0 {
			return UploadResult{}, errors.New("source file ended before expected size")
		}
		chunk := append([]byte(nil), buffer[:n]...)
		sum := sha256.Sum256(chunk)
		chunkSHA := hex.EncodeToString(sum[:])

		var updated UploadSession
		err = retry(ctx, retries, func() error {
			current, err := c.uploadChunk(ctx, folderID, session.ID, offset, chunk, chunkSHA)
			if err != nil {
				var httpErr *HTTPError
				if errors.As(err, &httpErr) && httpErr.Status == http.StatusConflict {
					refreshed, refreshErr := c.getUpload(ctx, folderID, session.ID)
					if refreshErr != nil {
						return err
					}
					if refreshed.ReceivedBytes == offset+int64(n) {
						updated = refreshed
						return nil
					}
				}
				return err
			}
			updated = current
			return nil
		})
		if err != nil {
			return UploadResult{}, fmt.Errorf("upload chunk at offset %d: %w", offset, err)
		}

		if updated.ReceivedBytes != offset+int64(n) {
			return UploadResult{}, fmt.Errorf(
				"server upload offset = %d, want %d",
				updated.ReceivedBytes,
				offset+int64(n),
			)
		}
		offset = updated.ReceivedBytes
		reportProgress(options.Progress, destination, offset, info.Size(), resumed)
	}

	result, err := c.completeUpload(ctx, folderID, session.ID)
	if err != nil {
		return UploadResult{}, err
	}
	if result.SizeBytes != info.Size() {
		return UploadResult{}, fmt.Errorf("server file size = %d, want %d", result.SizeBytes, info.Size())
	}
	if !strings.EqualFold(result.SHA256, fullSHA) {
		return UploadResult{}, errors.New("server SHA-256 does not match local file")
	}
	return result, nil
}

func (c *Client) listUploads(ctx context.Context, folderID string) ([]UploadSession, error) {
	var response struct {
		Uploads []UploadSession `json:"uploads"`
	}
	path := "/api/v1/files/folders/" + url.PathEscape(folderID) + "/uploads"
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &response, true); err != nil {
		return nil, err
	}
	return response.Uploads, nil
}

func (c *Client) createUpload(
	ctx context.Context,
	folderID, path string,
	totalBytes int64,
	expectedSHA, fingerprint string,
) (UploadSession, error) {
	var response struct {
		Upload UploadSession `json:"upload"`
	}
	payload := map[string]any{
		"path": path,
		"total_bytes": totalBytes,
		"sha256": expectedSHA,
		"client_fingerprint": fingerprint,
	}
	endpoint := "/api/v1/files/folders/" + url.PathEscape(folderID) + "/uploads"
	if err := c.doJSON(ctx, http.MethodPost, endpoint, payload, &response, true); err != nil {
		return UploadSession{}, err
	}
	return response.Upload, nil
}

func (c *Client) getUpload(ctx context.Context, folderID, uploadID string) (UploadSession, error) {
	var response struct {
		Upload UploadSession `json:"upload"`
	}
	endpoint := "/api/v1/files/folders/" + url.PathEscape(folderID) +
		"/uploads/" + url.PathEscape(uploadID)
	if err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response, true); err != nil {
		return UploadSession{}, err
	}
	return response.Upload, nil
}

func (c *Client) cancelUpload(ctx context.Context, folderID, uploadID string) error {
	endpoint := "/api/v1/files/folders/" + url.PathEscape(folderID) +
		"/uploads/" + url.PathEscape(uploadID)
	return c.doJSON(ctx, http.MethodDelete, endpoint, nil, nil, true)
}

func (c *Client) uploadChunk(
	ctx context.Context,
	folderID, uploadID string,
	offset int64,
	chunk []byte,
	chunkSHA string,
) (UploadSession, error) {
	endpoint := "/api/v1/files/folders/" + url.PathEscape(folderID) +
		"/uploads/" + url.PathEscape(uploadID) + "/chunk"
	request, err := c.newRequest(ctx, http.MethodPut, endpoint, bytes.NewReader(chunk), true)
	if err != nil {
		return UploadSession{}, err
	}
	request.Header.Set("Content-Type", "application/octet-stream")
	request.Header.Set("Upload-Offset", strconv.FormatInt(offset, 10))
	request.Header.Set("X-Chunk-SHA256", chunkSHA)

	response, err := c.HTTPClient.Do(request)
	if err != nil {
		return UploadSession{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return UploadSession{}, decodeHTTPError(response)
	}
	var payload struct {
		Upload UploadSession `json:"upload"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return UploadSession{}, err
	}
	return payload.Upload, nil
}

func (c *Client) completeUpload(ctx context.Context, folderID, uploadID string) (UploadResult, error) {
	var response struct {
		File UploadResult `json:"file"`
	}
	endpoint := "/api/v1/files/folders/" + url.PathEscape(folderID) +
		"/uploads/" + url.PathEscape(uploadID) + "/complete"
	if err := c.doJSON(ctx, http.MethodPost, endpoint, nil, &response, true); err != nil {
		return UploadResult{}, err
	}
	return response.File, nil
}

func (c *Client) doJSON(
	ctx context.Context,
	method, endpoint string,
	requestBody any,
	responseBody any,
	auth bool,
) error {
	var body io.Reader
	if requestBody != nil {
		data, err := json.Marshal(requestBody)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	request, err := c.newRequest(ctx, method, endpoint, body, auth)
	if err != nil {
		return err
	}
	if requestBody != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.HTTPClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return decodeHTTPError(response)
	}
	if responseBody == nil || response.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.NewDecoder(response.Body).Decode(responseBody)
}

func (c *Client) newRequest(
	ctx context.Context,
	method, endpoint string,
	body io.Reader,
	auth bool,
) (*http.Request, error) {
	relative, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	target := c.BaseURL.ResolveReference(relative)
	request, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	if auth {
		if strings.TrimSpace(c.Token) == "" {
			return nil, errors.New("client is not authenticated")
		}
		request.Header.Set("Authorization", "Bearer "+c.Token)
	}
	return request, nil
}

func decodeHTTPError(response *http.Response) error {
	payload := apiErrorBody{}
	_ = json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&payload)
	return &HTTPError{
		Status: response.StatusCode,
		Code: payload.Error.Code,
		Message: payload.Error.Message,
	}
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open source for checksum: %w", err)
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", fmt.Errorf("hash source file: %w", err)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func verifyUploadedChunks(path string, chunks []UploadChunk) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()
	var expectedOffset int64
	for _, chunk := range chunks {
		if chunk.Offset != expectedOffset || chunk.Size <= 0 {
			return false, nil
		}
		data := make([]byte, int(chunk.Size))
		if _, err := file.ReadAt(data, chunk.Offset); err != nil {
			return false, nil
		}
		sum := sha256.Sum256(data)
		if !strings.EqualFold(hex.EncodeToString(sum[:]), chunk.SHA256) {
			return false, nil
		}
		expectedOffset += chunk.Size
	}
	return true, nil
}

func fileFingerprint(info os.FileInfo) string {
	return fmt.Sprintf("windows-client:%s:%d:%d",
		info.Name(),
		info.Size(),
		info.ModTime().UTC().UnixNano(),
	)
}

func reportProgress(callback func(Progress), path string, uploaded, total int64, resumed bool) {
	if callback == nil {
		return
	}
	callback(Progress{
		Path: path,
		UploadedBytes: uploaded,
		TotalBytes: total,
		Resumed: resumed,
	})
}

func retry(ctx context.Context, attempts int, operation func() error) error {
	var last error
	for attempt := 0; attempt < attempts; attempt++ {
		if err := operation(); err != nil {
			last = err
			var httpErr *HTTPError
			if errors.As(err, &httpErr) && httpErr.Status >= 400 && httpErr.Status < 500 &&
				httpErr.Status != http.StatusRequestTimeout &&
				httpErr.Status != http.StatusTooManyRequests {
				return err
			}
			if attempt+1 < attempts {
				delay := time.Duration(attempt+1) * 500 * time.Millisecond
				timer := time.NewTimer(delay)
				select {
				case <-ctx.Done():
					timer.Stop()
					return ctx.Err()
				case <-timer.C:
				}
			}
			continue
		}
		return nil
	}
	return last
}
