package nvr

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

var (
	ErrLiveUnavailable = errors.New("NVR live runtime is unavailable")
	ErrCameraDisabled  = errors.New("camera is disabled")
)

type LiveSource interface {
	Available() bool
	Start(context.Context, ProbeRequest) (io.ReadCloser, <-chan error, error)
}

type FFmpegMJPEGSource struct {
	path string
}

func NewFFmpegMJPEGSource() *FFmpegMJPEGSource {
	path, _ := exec.LookPath("ffmpeg")
	return &FFmpegMJPEGSource{path: path}
}

func (s *FFmpegMJPEGSource) Available() bool {
	return s != nil && s.path != ""
}

func (s *FFmpegMJPEGSource) Start(
	ctx context.Context,
	request ProbeRequest,
) (io.ReadCloser, <-chan error, error) {
	if !s.Available() {
		return nil, nil, ErrLiveUnavailable
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

	command := exec.CommandContext(
		ctx,
		s.path,
		"-hide_banner",
		"-loglevel", "error",
		"-f", "concat",
		"-safe", "0",
		"-protocol_whitelist", "file,pipe,rtsp,tcp,udp,rtp,tls,http,https,crypto",
		"-i", "pipe:0",
		"-map", "0:v:0",
		"-an",
		"-vf", "scale=w='min(1280,iw)':h=-2,fps=5",
		"-q:v", "6",
		"-f", "image2pipe",
		"-vcodec", "mjpeg",
		"pipe:1",
	)
	command.Stdin = strings.NewReader(ffconcatRTSPInput(sourceURL, transport))
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, nil, ErrLiveUnavailable
	}
	if err := command.Start(); err != nil {
		return nil, nil, ErrLiveUnavailable
	}

	done := make(chan error, 1)
	go func() {
		err := command.Wait()
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		done <- err
		close(done)
	}()
	return stdout, done, nil
}

type liveSession struct {
	cameraID string
	cancel   context.CancelFunc
	done     chan struct{}

	mu          sync.RWMutex
	latest      []byte
	sequence    uint64
	notify      chan struct{}
	closed      bool
	lastError   error
	subscribers int
}

type LiveSubscription struct {
	service *Service
	session *liveSession
	seq     uint64
	once    sync.Once
}

func (s *Service) LiveReady() bool {
	return s != nil && s.liveSource != nil && s.liveSource.Available()
}

func (s *Service) ActiveLiveStreams() int {
	if s == nil {
		return 0
	}
	s.liveMu.Lock()
	defer s.liveMu.Unlock()
	return len(s.liveSessions)
}

func (s *Service) SubscribeLive(ctx context.Context, cameraID string) (*LiveSubscription, error) {
	if s == nil || s.liveSource == nil || !s.liveSource.Available() {
		return nil, ErrLiveUnavailable
	}
	cameraID = strings.TrimSpace(cameraID)
	camera, err := s.store.NVRCamera(ctx, cameraID)
	if err != nil {
		return nil, err
	}
	if !camera.Enabled {
		return nil, ErrCameraDisabled
	}
	credential, err := s.resolveCredential(ctx, camera.CredentialRef)
	if err != nil {
		return nil, err
	}

	s.liveMu.Lock()
	if session, ok := s.liveSessions[cameraID]; ok && !session.isClosed() {
		session.subscribers++
		s.liveMu.Unlock()
		return &LiveSubscription{service: s, session: session}, nil
	}

	liveCtx, cancel := context.WithCancel(context.Background())
	stdout, processDone, err := s.liveSource.Start(liveCtx, ProbeRequest{
		Address:    camera.Address,
		Transport:  camera.Transport,
		Credential: credential,
	})
	if err != nil {
		cancel()
		s.liveMu.Unlock()
		return nil, err
	}
	session := &liveSession{
		cameraID:     cameraID,
		cancel:       cancel,
		done:         make(chan struct{}),
		notify:       make(chan struct{}),
		subscribers:  1,
	}
	s.liveSessions[cameraID] = session
	s.liveMu.Unlock()

	go s.runLiveSession(session, stdout, processDone)
	return &LiveSubscription{service: s, session: session}, nil
}

func (s *LiveSubscription) Next(ctx context.Context) ([]byte, error) {
	if s == nil || s.session == nil {
		return nil, ErrLiveUnavailable
	}
	for {
		s.session.mu.RLock()
		if s.session.sequence > s.seq && len(s.session.latest) > 0 {
			s.seq = s.session.sequence
			frame := s.session.latest
			s.session.mu.RUnlock()
			return frame, nil
		}
		if s.session.closed {
			err := s.session.lastError
			s.session.mu.RUnlock()
			if err == nil || errors.Is(err, context.Canceled) {
				return nil, io.EOF
			}
			return nil, ErrLiveUnavailable
		}
		notify := s.session.notify
		s.session.mu.RUnlock()

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-notify:
		}
	}
}

func (s *LiveSubscription) Close() {
	if s == nil || s.service == nil || s.session == nil {
		return
	}
	s.once.Do(func() {
		s.service.releaseLive(s.session)
	})
}

func (s *Service) runLiveSession(
	session *liveSession,
	stdout io.ReadCloser,
	processDone <-chan error,
) {
	defer stdout.Close()

	reader := bufio.NewReaderSize(stdout, 256*1024)
	var streamErr error
	for {
		frame, err := readJPEGFrame(reader, 8<<20)
		if err != nil {
			streamErr = err
			break
		}
		session.publish(frame)
	}

	select {
	case err := <-processDone:
		if err != nil && !errors.Is(err, context.Canceled) {
			streamErr = err
		}
	default:
	}
	session.finish(streamErr)

	s.liveMu.Lock()
	if current := s.liveSessions[session.cameraID]; current == session {
		delete(s.liveSessions, session.cameraID)
	}
	s.liveMu.Unlock()
}

func (s *Service) releaseLive(session *liveSession) {
	s.liveMu.Lock()
	current := s.liveSessions[session.cameraID]
	if current != session {
		s.liveMu.Unlock()
		return
	}
	if session.subscribers > 0 {
		session.subscribers--
	}
	if session.subscribers != 0 {
		s.liveMu.Unlock()
		return
	}
	delay := s.liveIdleTimeout
	if delay <= 0 {
		delay = 5 * time.Second
	}
	s.liveMu.Unlock()

	go func() {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		<-timer.C

		s.liveMu.Lock()
		current := s.liveSessions[session.cameraID]
		if current == session && session.subscribers == 0 {
			delete(s.liveSessions, session.cameraID)
			session.cancel()
		}
		s.liveMu.Unlock()
	}()
}

func (s *Service) StopLive(cameraID string) {
	if s == nil {
		return
	}
	s.liveMu.Lock()
	session := s.liveSessions[strings.TrimSpace(cameraID)]
	if session != nil {
		delete(s.liveSessions, session.cameraID)
	}
	s.liveMu.Unlock()
	if session != nil {
		session.cancel()
	}
}

func (s *Service) stopAllLive() {
	if s == nil {
		return
	}
	s.liveMu.Lock()
	sessions := make([]*liveSession, 0, len(s.liveSessions))
	for id, session := range s.liveSessions {
		sessions = append(sessions, session)
		delete(s.liveSessions, id)
	}
	s.liveMu.Unlock()
	for _, session := range sessions {
		session.cancel()
	}
}

func (s *liveSession) isClosed() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.closed
}

func (s *liveSession) publish(frame []byte) {
	if len(frame) == 0 {
		return
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.latest = frame
	s.sequence++
	close(s.notify)
	s.notify = make(chan struct{})
	s.mu.Unlock()
}

func (s *liveSession) finish(err error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.lastError = err
	close(s.notify)
	close(s.done)
	s.mu.Unlock()
}

func readJPEGFrame(reader *bufio.Reader, maxBytes int) ([]byte, error) {
	if maxBytes <= 0 {
		maxBytes = 8 << 20
	}
	frame := make([]byte, 0, 128*1024)
	previous := byte(0)
	started := false
	for {
		value, err := reader.ReadByte()
		if err != nil {
			return nil, err
		}
		if !started {
			if previous == 0xff && value == 0xd8 {
				frame = append(frame, 0xff, 0xd8)
				started = true
			}
			previous = value
			continue
		}
		frame = append(frame, value)
		if len(frame) > maxBytes {
			return nil, errors.New("live JPEG frame exceeds size limit")
		}
		if previous == 0xff && value == 0xd9 {
			return frame, nil
		}
		previous = value
	}
}
