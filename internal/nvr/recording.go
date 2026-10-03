package nvr

import (
	"bufio"
	"context"
	"encoding/csv"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/state"
)

var (
	ErrRecordingUnavailable = errors.New("NVR recording runtime is unavailable")
	ErrRecordingNoStorage   = errors.New("NVR recording storage is not configured")
)

type RecordedSegment struct {
	Path        string
	StartOffset time.Duration
	EndOffset   time.Duration
}

type RecorderSource interface {
	Available() bool
	Start(context.Context, ProbeRequest, string) (<-chan RecordedSegment, <-chan error, error)
}

type FFmpegSegmentRecorder struct {
	path            string
	segmentDuration time.Duration
}

func NewFFmpegSegmentRecorder() *FFmpegSegmentRecorder {
	path, _ := exec.LookPath("ffmpeg")
	return &FFmpegSegmentRecorder{path: path, segmentDuration: 60 * time.Second}
}

func (r *FFmpegSegmentRecorder) Available() bool {
	return r != nil && r.path != ""
}

func (r *FFmpegSegmentRecorder) Start(
	ctx context.Context,
	request ProbeRequest,
	outputDir string,
) (<-chan RecordedSegment, <-chan error, error) {
	if !r.Available() {
		return nil, nil, ErrRecordingUnavailable
	}
	address, err := normalizeRTSPAddress(request.Address)
	if err != nil {
		return nil, nil, err
	}
	transport, err := normalizeTransport(request.Transport)
	if err != nil {
		return nil, nil, err
	}
	sourceURL, err := credentialedRTSPAddress(address, request.Credential)
	if err != nil {
		return nil, nil, err
	}
	if err := os.MkdirAll(outputDir, 0o750); err != nil {
		return nil, nil, err
	}

	segmentSeconds := int(r.segmentDuration / time.Second)
	if segmentSeconds <= 0 {
		segmentSeconds = 60
	}
	pattern := filepath.Join(outputDir, "%Y%m%dT%H%M%SZ.mp4")
	command := exec.CommandContext(
		ctx,
		r.path,
		"-hide_banner",
		"-loglevel", "error",
		"-f", "concat",
		"-safe", "0",
		"-protocol_whitelist", "file,pipe,rtsp,tcp,udp,rtp,tls,http,https,crypto",
		"-i", "pipe:0",
		"-map", "0:v:0",
		"-map", "0:a?",
		"-c", "copy",
		"-f", "segment",
		"-segment_time", strconv.Itoa(segmentSeconds),
		"-reset_timestamps", "1",
		"-strftime", "1",
		"-segment_list", "pipe:1",
		"-segment_list_type", "csv",
		pattern,
	)
	command.Stdin = strings.NewReader(ffconcatRTSPInput(sourceURL, transport))
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, nil, ErrRecordingUnavailable
	}
	if err := command.Start(); err != nil {
		return nil, nil, ErrRecordingUnavailable
	}

	segments := make(chan RecordedSegment, 4)
	done := make(chan error, 1)
	go func() {
		defer close(segments)
		reader := csv.NewReader(bufio.NewReader(stdout))
		reader.FieldsPerRecord = -1
		for {
			row, err := reader.Read()
			if err != nil {
				if !errors.Is(err, io.EOF) && ctx.Err() == nil {
					done <- err
				}
				break
			}
			if len(row) < 3 {
				continue
			}
			startSeconds, err1 := strconv.ParseFloat(strings.TrimSpace(row[1]), 64)
			endSeconds, err2 := strconv.ParseFloat(strings.TrimSpace(row[2]), 64)
			if err1 != nil || err2 != nil || endSeconds < startSeconds {
				continue
			}
			select {
			case <-ctx.Done():
				return
			case segments <- RecordedSegment{
				Path:        filepath.Clean(row[0]),
				StartOffset: time.Duration(startSeconds * float64(time.Second)),
				EndOffset:   time.Duration(endSeconds * float64(time.Second)),
			}:
			}
		}
	}()
	go func() {
		err := command.Wait()
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		select {
		case done <- err:
		default:
		}
		close(done)
	}()
	return segments, done, nil
}

type recordingSession struct {
	cancel context.CancelFunc
	done   chan struct{}
}

type RecordingStatus struct {
	Active          bool       `json:"active"`
	LastSegmentAt   *time.Time `json:"last_segment_at,omitempty"`
	LastSegmentPath string     `json:"last_segment_path,omitempty"`
	LastError       string     `json:"last_error,omitempty"`
}

func (s *Service) RecordingReady() bool {
	return s != nil && s.recorder != nil && s.recorder.Available()
}

func (s *Service) ActiveRecordings() int {
	if s == nil {
		return 0
	}
	s.recordingMu.Lock()
	defer s.recordingMu.Unlock()
	return len(s.recordingSessions)
}

func (s *Service) CameraRecording(cameraID string) RecordingStatus {
	if s == nil {
		return RecordingStatus{}
	}
	s.recordingMu.Lock()
	defer s.recordingMu.Unlock()
	return s.recordingStatus[strings.TrimSpace(cameraID)]
}

func (s *Service) startRecording(ctx context.Context, camera state.NVRCameraRecord) error {
	if !camera.Enabled || camera.RecordingMode != "continuous" {
		return nil
	}
	if !s.RecordingReady() {
		return ErrRecordingUnavailable
	}
	target, err := s.store.ActiveNVRStorageTarget(ctx)
	if err != nil {
		return ErrRecordingNoStorage
	}
	credential, err := s.resolveCredential(ctx, camera.CredentialRef)
	if err != nil {
		return err
	}
	profiles, err := s.store.ListNVRStreamProfiles(ctx, camera.ID)
	if err != nil {
		return err
	}
	var main state.NVRStreamProfileRecord
	for _, profile := range profiles {
		if profile.Role == "main" {
			main = profile
			break
		}
	}
	if main.SourceURI == "" {
		main.SourceURI = camera.Address
	}
	outputDir := filepath.Join(target.Mountpoint, "home-ai-nvr", camera.ID)
	recordingCtx, cancel := context.WithCancel(context.Background())
	startedAt := s.now().UTC()
	segments, processDone, err := s.recorder.Start(recordingCtx, ProbeRequest{
		Address:    main.SourceURI,
		Transport:  camera.Transport,
		Credential: credential,
	}, outputDir)
	if err != nil {
		cancel()
		return err
	}
	session := &recordingSession{cancel: cancel, done: make(chan struct{})}
	s.recordingMu.Lock()
	if current := s.recordingSessions[camera.ID]; current != nil {
		current.cancel()
	}
	s.recordingSessions[camera.ID] = session
	s.recordingStatus[camera.ID] = RecordingStatus{Active: true}
	s.recordingMu.Unlock()

	go s.runRecordingSession(session, camera, main, target, startedAt, segments, processDone)
	return nil
}

func (s *Service) runRecordingSession(
	session *recordingSession,
	camera state.NVRCameraRecord,
	profile state.NVRStreamProfileRecord,
	target state.NVRStorageTargetRecord,
	startedAt time.Time,
	segments <-chan RecordedSegment,
	processDone <-chan error,
) {
	defer close(session.done)
	for segment := range segments {
		info, err := os.Stat(segment.Path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		rel, err := filepath.Rel(target.Mountpoint, segment.Path)
		if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			continue
		}
		startAt := startedAt.Add(segment.StartOffset)
		endAt := startedAt.Add(segment.EndOffset)
		if endAt.Before(startAt) {
			continue
		}
		_, err = s.store.CreateNVRRecordingSegment(
			context.Background(),
			camera.ID,
			target.ID,
			startAt,
			endAt,
			filepath.ToSlash(rel),
			profile.Codec,
			profile.Width,
			profile.Height,
			info.Size(),
			false,
			"complete",
			s.now().UTC(),
		)
		if err != nil {
			s.setRecordingError(camera.ID, "recording segment index failed")
			continue
		}
		now := s.now().UTC()
		s.recordingMu.Lock()
		status := s.recordingStatus[camera.ID]
		status.Active = true
		status.LastSegmentAt = &now
		status.LastSegmentPath = filepath.ToSlash(rel)
		status.LastError = ""
		s.recordingStatus[camera.ID] = status
		s.recordingMu.Unlock()
	}
	var processErr error
	if processDone != nil {
		processErr = <-processDone
	}
	s.recordingMu.Lock()
	if current := s.recordingSessions[camera.ID]; current == session {
		delete(s.recordingSessions, camera.ID)
		status := s.recordingStatus[camera.ID]
		status.Active = false
		if processErr != nil && !errors.Is(processErr, context.Canceled) {
			status.LastError = "recording process stopped"
		}
		s.recordingStatus[camera.ID] = status
	}
	s.recordingMu.Unlock()
}

func (s *Service) setRecordingError(cameraID, message string) {
	s.recordingMu.Lock()
	status := s.recordingStatus[cameraID]
	status.LastError = message
	s.recordingStatus[cameraID] = status
	s.recordingMu.Unlock()
}

func (s *Service) StopRecording(cameraID string) {
	if s == nil {
		return
	}
	cameraID = strings.TrimSpace(cameraID)
	s.recordingMu.Lock()
	session := s.recordingSessions[cameraID]
	if session != nil {
		delete(s.recordingSessions, cameraID)
	}
	status := s.recordingStatus[cameraID]
	status.Active = false
	s.recordingStatus[cameraID] = status
	s.recordingMu.Unlock()
	if session != nil {
		session.cancel()
	}
}

func (s *Service) stopAllRecordings() {
	if s == nil {
		return
	}
	s.recordingMu.Lock()
	sessions := make([]*recordingSession, 0, len(s.recordingSessions))
	for id, session := range s.recordingSessions {
		sessions = append(sessions, session)
		delete(s.recordingSessions, id)
		status := s.recordingStatus[id]
		status.Active = false
		s.recordingStatus[id] = status
	}
	s.recordingMu.Unlock()
	for _, session := range sessions {
		session.cancel()
	}
}

var _ = sync.Mutex{}
