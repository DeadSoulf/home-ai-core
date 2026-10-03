package nvr

import (
	"bufio"
	"context"
	"encoding/csv"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/state"
)

var (
	ErrRecordingUnavailable      = errors.New("NVR recording runtime is unavailable")
	ErrRecordingNoStorage        = errors.New("NVR recording storage is not configured")
	ErrRecordingStorageFull      = errors.New("NVR recording storage reserve cannot be restored")
	ErrRecordingStorageUnmounted = errors.New("NVR recording storage is not mounted")
)

type RecordedSegment struct {
	Path        string
	StartOffset time.Duration
	EndOffset   time.Duration
}

type RecorderSource interface {
	Available() bool
	Start(context.Context, ProbeRequest, string, bool) (<-chan RecordedSegment, <-chan error, error)
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
	audioEnabled bool,
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
	pattern := filepath.Join(outputDir, "%Y%m%dT%H%M%SZ.partial.mp4")
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-f", "concat",
		"-safe", "0",
		"-protocol_whitelist", "file,pipe,rtsp,tcp,udp,rtp,tls,http,https,crypto",
		"-i", "pipe:0",
		"-map", "0:v:0",
	}
	if audioEnabled {
		args = append(args, "-map", "0:a?")
	} else {
		args = append(args, "-an")
	}
	args = append(args,
		"-c", "copy",
		"-f", "segment",
		"-segment_time", strconv.Itoa(segmentSeconds),
		"-reset_timestamps", "1",
		"-strftime", "1",
		"-segment_list", "pipe:1",
		"-segment_list_type", "csv",
		pattern,
	)
	command := exec.CommandContext(ctx, r.path, args...)
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

type recordingAttempt struct {
	cancel      context.CancelFunc
	startedAt   time.Time
	segments    <-chan RecordedSegment
	processDone <-chan error
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
	active := 0
	for _, status := range s.recordingStatus {
		if status.Active {
			active++
		}
	}
	return active
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
	if err := s.ensureRecordingMount(target); err != nil {
		return err
	}
	if err := s.enforceStorageReserve(ctx, target); err != nil {
		return err
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

	recordingCtx, cancel := context.WithCancel(context.Background())
	attempt, err := s.startRecordingAttempt(
		recordingCtx,
		camera,
		main,
		target,
		credential,
	)
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

	go s.runRecordingWorker(
		recordingCtx,
		session,
		camera,
		main,
		target,
		credential,
		attempt,
	)
	return nil
}

func (s *Service) startRecordingAttempt(
	ctx context.Context,
	camera state.NVRCameraRecord,
	profile state.NVRStreamProfileRecord,
	target state.NVRStorageTargetRecord,
	credential CameraCredential,
) (recordingAttempt, error) {
	if err := s.ensureRecordingMount(target); err != nil {
		return recordingAttempt{}, err
	}
	if err := s.enforceStorageReserve(ctx, target); err != nil {
		return recordingAttempt{}, err
	}

	cameraRoot := filepath.Join(target.Mountpoint, "home-ai-nvr", camera.ID)
	if err := cleanupPartialSegments(cameraRoot); err != nil {
		return recordingAttempt{}, err
	}
	startedAt := s.now().UTC()
	outputDir := filepath.Join(cameraRoot, strconv.FormatInt(startedAt.UnixNano(), 10))

	attemptCtx, attemptCancel := context.WithCancel(ctx)
	segments, processDone, err := s.recorder.Start(attemptCtx, ProbeRequest{
		Address:    profile.SourceURI,
		Transport:  camera.Transport,
		Credential: credential,
	}, outputDir, camera.AudioEnabled)
	if err != nil {
		attemptCancel()
		return recordingAttempt{}, err
	}
	return recordingAttempt{
		cancel:      attemptCancel,
		startedAt:   startedAt,
		segments:    segments,
		processDone: processDone,
	}, nil
}

func (s *Service) runRecordingWorker(
	ctx context.Context,
	session *recordingSession,
	camera state.NVRCameraRecord,
	profile state.NVRStreamProfileRecord,
	target state.NVRStorageTargetRecord,
	credential CameraCredential,
	attempt recordingAttempt,
) {
	defer close(session.done)

	retryIndex := 0
	for {
		hadSegment, processErr := s.consumeRecordingAttempt(
			session,
			camera,
			profile,
			target,
			attempt,
		)
		attempt.cancel()

		if ctx.Err() != nil {
			break
		}
		if errors.Is(processErr, ErrRecordingStorageFull) {
			s.setRecordingState(camera.ID, false, runtimeRecordingErrorMessage(processErr))
			break
		}
		if processErr == nil {
			processErr = errors.New("recording process stopped")
		}
		s.setRecordingState(camera.ID, false, "recording process stopped")

		if hadSegment {
			retryIndex = 0
		}
		if !waitRuntime(ctx, s.retryDelay(retryIndex)) {
			break
		}
		if retryIndex < len(s.retryDelays)-1 {
			retryIndex++
		}

		for {
			next, err := s.startRecordingAttempt(ctx, camera, profile, target, credential)
			if err == nil {
				attempt = next
				s.setRecordingState(camera.ID, true, "")
				break
			}
			if ctx.Err() != nil {
				break
			}
			if errors.Is(err, ErrRecordingStorageFull) {
				s.setRecordingState(camera.ID, false, runtimeRecordingErrorMessage(err))
				processErr = err
				break
			}
			s.setRecordingState(camera.ID, false, runtimeRecordingErrorMessage(err))
			if !waitRuntime(ctx, s.retryDelay(retryIndex)) {
				break
			}
			if retryIndex < len(s.retryDelays)-1 {
				retryIndex++
			}
		}
		if ctx.Err() != nil || errors.Is(processErr, ErrRecordingStorageFull) {
			break
		}
	}

	s.recordingMu.Lock()
	if current := s.recordingSessions[camera.ID]; current == session {
		delete(s.recordingSessions, camera.ID)
		status := s.recordingStatus[camera.ID]
		status.Active = false
		s.recordingStatus[camera.ID] = status
	}
	s.recordingMu.Unlock()
}

func (s *Service) consumeRecordingAttempt(
	session *recordingSession,
	camera state.NVRCameraRecord,
	profile state.NVRStreamProfileRecord,
	target state.NVRStorageTargetRecord,
	attempt recordingAttempt,
) (bool, error) {
	hadSegment := false
	for segment := range attempt.segments {
		segmentPath, err := finalizeRecordedSegment(segment.Path)
		if err != nil {
			s.setRecordingError(camera.ID, "recording segment finalize failed")
			continue
		}
		info, err := os.Stat(segmentPath)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		rel, err := filepath.Rel(target.Mountpoint, segmentPath)
		if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			continue
		}
		startAt := attempt.startedAt.Add(segment.StartOffset)
		endAt := attempt.startedAt.Add(segment.EndOffset)
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
		hadSegment = true
		if err := s.enforceStorageReserve(context.Background(), target); err != nil {
			attempt.cancel()
			return hadSegment, err
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
	if attempt.processDone == nil {
		return hadSegment, nil
	}
	processErr := <-attempt.processDone
	if errors.Is(processErr, context.Canceled) && session != nil {
		return hadSegment, processErr
	}
	return hadSegment, processErr
}

func (s *Service) setRecordingState(cameraID string, active bool, message string) {
	s.recordingMu.Lock()
	status := s.recordingStatus[cameraID]
	status.Active = active
	status.LastError = message
	s.recordingStatus[cameraID] = status
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
		stopRecordingSession(session)
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
		stopRecordingSession(session)
	}
}

func stopRecordingSession(session *recordingSession) {
	if session == nil {
		return
	}
	session.cancel()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	select {
	case <-session.done:
	case <-timer.C:
	}
}

func (s *Service) RefreshRecordings(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.stopAllRecordings()
	cameras, err := s.store.ListNVRCameras(ctx)
	if err != nil {
		return err
	}
	for _, camera := range cameras {
		if !camera.Enabled || camera.RecordingMode != "continuous" {
			continue
		}
		if err := s.startRecording(ctx, camera); err != nil {
			s.setRecordingError(camera.ID, runtimeRecordingErrorMessage(err))
		}
	}
	return nil
}

func (s *Service) ensureRecordingMount(target state.NVRStorageTargetRecord) error {
	mountpoint := filepath.Clean(strings.TrimSpace(target.Mountpoint))
	if mountpoint == "." || mountpoint == "" || mountpoint == string(filepath.Separator) || !filepath.IsAbs(mountpoint) {
		return ErrRecordingStorageUnmounted
	}
	if s == nil || s.mountChecker == nil {
		return ErrRecordingStorageUnmounted
	}
	mounted, err := s.mountChecker.Mounted(mountpoint)
	if err != nil {
		return err
	}
	if !mounted {
		return ErrRecordingStorageUnmounted
	}
	return nil
}

func (s *Service) enforceStorageReserve(
	ctx context.Context,
	target state.NVRStorageTargetRecord,
) error {
	if s == nil || s.spaceChecker == nil {
		return nil
	}
	for {
		space, err := s.spaceChecker.Space(target.Mountpoint)
		if err != nil {
			return err
		}
		if space.TotalBytes == 0 {
			return nil
		}
		reserveBytes := space.TotalBytes * uint64(target.ReservePercent) / 100
		if space.AvailableBytes >= reserveBytes {
			return nil
		}

		candidates, err := s.store.OldestNVRRetentionSegments(ctx, target.ID, 32)
		if err != nil {
			return err
		}
		if len(candidates) == 0 {
			return ErrRecordingStorageFull
		}

		deletedAny := false
		for _, candidate := range candidates {
			fullPath := filepath.Join(target.Mountpoint, filepath.FromSlash(candidate.RelativePath))
			rel, err := filepath.Rel(target.Mountpoint, fullPath)
			if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
				continue
			}
			if err := os.Remove(fullPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err := s.store.DeleteNVRRecordingSegment(ctx, candidate.ID); err != nil {
				return err
			}
			deletedAny = true

			space, err = s.spaceChecker.Space(target.Mountpoint)
			if err != nil {
				return err
			}
			if space.AvailableBytes >= reserveBytes {
				return nil
			}
		}
		if !deletedAny {
			return ErrRecordingStorageFull
		}
	}
}

func finalizeRecordedSegment(path string) (string, error) {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "." || path == "" {
		return "", errors.New("recording segment path is empty")
	}
	const partialSuffix = ".partial.mp4"
	if !strings.HasSuffix(strings.ToLower(path), partialSuffix) {
		return path, nil
	}
	finalPath := path[:len(path)-len(partialSuffix)] + ".mp4"
	if _, err := os.Stat(finalPath); err == nil {
		finalPath = path[:len(path)-len(partialSuffix)] + "-" + strconv.FormatInt(time.Now().UTC().UnixNano(), 10) + ".mp4"
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.Rename(path, finalPath); err != nil {
		return "", err
	}
	return finalPath, nil
}

func cleanupPartialSegments(root string) error {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "." || root == "" {
		return nil
	}
	if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(entry.Name()), ".partial.mp4") {
			return nil
		}
		return os.Remove(path)
	})
}
