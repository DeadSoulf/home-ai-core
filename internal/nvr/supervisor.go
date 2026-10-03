package nvr

import (
	"context"
	"errors"
	"time"
)

const (
	RuntimeDisabled   = "disabled"
	RuntimeConnecting = "connecting"
	RuntimeOnline     = "online"
	RuntimeOffline    = "offline"
)

type CameraRuntimeStatus struct {
	State          string     `json:"state"`
	LastSeenAt     *time.Time `json:"last_seen_at,omitempty"`
	LastCheckedAt  *time.Time `json:"last_checked_at,omitempty"`
	LastError      string     `json:"last_error,omitempty"`
	ReconnectCount int        `json:"reconnect_count"`
}

type RuntimeEvent struct {
	CameraID      string
	State         string
	PreviousState string
	Status        CameraRuntimeStatus
}

type cameraWorker struct {
	cancel context.CancelFunc
}

func (s *Service) SetRuntimeEventHandler(handler func(RuntimeEvent)) {
	s.runtimeMu.Lock()
	s.runtimeEvent = handler
	s.runtimeMu.Unlock()
}

func (s *Service) SupervisorRunning() bool {
	if s == nil {
		return false
	}
	s.runtimeMu.RLock()
	defer s.runtimeMu.RUnlock()
	return s.supervisorCancel != nil
}

func (s *Service) Start(ctx context.Context) error {
	if s == nil {
		return errors.New("NVR service is unavailable")
	}
	cameras, err := s.store.ListNVRCameras(ctx)
	if err != nil {
		return err
	}

	s.runtimeMu.Lock()
	if s.supervisorCancel != nil {
		s.runtimeMu.Unlock()
		return nil
	}
	rootCtx, cancel := context.WithCancel(context.Background())
	s.supervisorCancel = cancel
	for _, camera := range cameras {
		if camera.Enabled {
			s.startWorkerLocked(rootCtx, camera.ID)
		} else {
			s.runtime[camera.ID] = CameraRuntimeStatus{State: RuntimeDisabled}
		}
	}
	s.runtimeMu.Unlock()

	for _, camera := range cameras {
		if camera.Enabled && camera.RecordingMode == "continuous" {
			if err := s.startRecording(ctx, camera); err != nil {
				s.setRecordingError(camera.ID, runtimeRecordingErrorMessage(err))
			}
		}
	}
	return nil
}

func (s *Service) Stop() {
	if s == nil {
		return
	}
	s.stopAllLive()
	s.stopAllRecordings()
	s.runtimeMu.Lock()
	cancel := s.supervisorCancel
	s.supervisorCancel = nil
	for id, worker := range s.workers {
		worker.cancel()
		delete(s.workers, id)
		current := s.runtime[id]
		current.State = RuntimeDisabled
		current.LastError = ""
		s.runtime[id] = current
	}
	s.runtimeMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (s *Service) Restart(ctx context.Context) error {
	s.Stop()
	return s.Start(ctx)
}

func (s *Service) RefreshCamera(ctx context.Context, cameraID string) error {
	if s == nil {
		return nil
	}
	s.StopLive(cameraID)
	s.StopRecording(cameraID)
	camera, err := s.store.NVRCamera(ctx, cameraID)
	if err != nil {
		return err
	}

	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()
	if worker, ok := s.workers[cameraID]; ok {
		worker.cancel()
		delete(s.workers, cameraID)
	}
	if s.supervisorCancel == nil || !camera.Enabled {
		s.runtime[cameraID] = CameraRuntimeStatus{State: RuntimeDisabled}
		return nil
	}
	// Workers are intentionally rooted in their own context so an HTTP request
	// finishing cannot stop camera supervision.
	workerCtx, cancel := context.WithCancel(context.Background())
	s.workers[cameraID] = cameraWorker{cancel: cancel}
	s.runtime[cameraID] = CameraRuntimeStatus{State: RuntimeConnecting}
	go s.runCameraWorker(workerCtx, cameraID)
	if camera.RecordingMode == "continuous" {
		if err := s.startRecording(ctx, camera); err != nil {
			s.setRecordingError(camera.ID, runtimeRecordingErrorMessage(err))
		}
	}
	return nil
}

func (s *Service) RemoveCameraRuntime(cameraID string) {
	if s == nil {
		return
	}
	s.StopLive(cameraID)
	s.StopRecording(cameraID)
	s.runtimeMu.Lock()
	if worker, ok := s.workers[cameraID]; ok {
		worker.cancel()
		delete(s.workers, cameraID)
	}
	delete(s.runtime, cameraID)
	s.runtimeMu.Unlock()
}

func (s *Service) CameraRuntime(cameraID string) CameraRuntimeStatus {
	if s == nil {
		return CameraRuntimeStatus{State: RuntimeDisabled}
	}
	s.runtimeMu.RLock()
	defer s.runtimeMu.RUnlock()
	status, ok := s.runtime[cameraID]
	if !ok {
		return CameraRuntimeStatus{State: RuntimeDisabled}
	}
	return status
}

func (s *Service) RuntimeCounts() (online, offline int) {
	if s == nil {
		return 0, 0
	}
	s.runtimeMu.RLock()
	defer s.runtimeMu.RUnlock()
	for _, status := range s.runtime {
		switch status.State {
		case RuntimeOnline:
			online++
		case RuntimeOffline:
			offline++
		}
	}
	return online, offline
}

func (s *Service) startWorkerLocked(rootCtx context.Context, cameraID string) {
	workerCtx, cancel := context.WithCancel(rootCtx)
	s.workers[cameraID] = cameraWorker{cancel: cancel}
	s.runtime[cameraID] = CameraRuntimeStatus{State: RuntimeConnecting}
	go s.runCameraWorker(workerCtx, cameraID)
}

func (s *Service) runCameraWorker(ctx context.Context, cameraID string) {
	retryIndex := 0
	hadSuccessfulProbe := false
	for {
		if ctx.Err() != nil {
			return
		}

		_, err := s.TestExistingCamera(ctx, cameraID)
		now := s.now().UTC()
		if err == nil {
			retryIndex = 0
			hadSuccessfulProbe = true
			s.updateRuntime(cameraID, func(current CameraRuntimeStatus) CameraRuntimeStatus {
				current.State = RuntimeOnline
				current.LastSeenAt = timePtr(now)
				current.LastCheckedAt = timePtr(now)
				current.LastError = ""
				return current
			})
			if !waitRuntime(ctx, s.healthInterval) {
				return
			}
			continue
		}
		if ctx.Err() != nil {
			return
		}

		safeError := runtimeErrorMessage(err)
		s.updateRuntime(cameraID, func(current CameraRuntimeStatus) CameraRuntimeStatus {
			current.State = RuntimeOffline
			current.LastCheckedAt = timePtr(now)
			current.LastError = safeError
			if hadSuccessfulProbe || current.ReconnectCount > 0 {
				current.ReconnectCount++
			} else {
				current.ReconnectCount = 1
			}
			return current
		})

		delay := s.retryDelay(retryIndex)
		if retryIndex < len(s.retryDelays)-1 {
			retryIndex++
		}
		if !waitRuntime(ctx, delay) {
			return
		}
	}
}

func (s *Service) updateRuntime(cameraID string, mutate func(CameraRuntimeStatus) CameraRuntimeStatus) {
	s.runtimeMu.Lock()
	current := s.runtime[cameraID]
	previous := current.State
	updated := mutate(current)
	s.runtime[cameraID] = updated
	handler := s.runtimeEvent
	s.runtimeMu.Unlock()

	if handler != nil && updated.State != previous {
		handler(RuntimeEvent{
			CameraID:      cameraID,
			State:         updated.State,
			PreviousState: previous,
			Status:        updated,
		})
	}
}

func (s *Service) retryDelay(index int) time.Duration {
	if len(s.retryDelays) == 0 {
		return 5 * time.Second
	}
	if index < 0 {
		index = 0
	}
	if index >= len(s.retryDelays) {
		index = len(s.retryDelays) - 1
	}
	return s.retryDelays[index]
}

func runtimeErrorMessage(err error) string {
	switch {
	case errors.Is(err, ErrRTSPAuthentication):
		return "camera authentication failed"
	case errors.Is(err, ErrRTSPConnection):
		return "camera connection failed"
	case errors.Is(err, ErrRTSPNoVideo):
		return "camera source has no video stream"
	case errors.Is(err, ErrMediaRuntimeUnavailable):
		return "NVR media runtime is unavailable"
	case errors.Is(err, ErrSecretStoreUnavailable):
		return "camera credential store is unavailable"
	case errors.Is(err, context.DeadlineExceeded):
		return "camera health check timed out"
	default:
		return "camera health check failed"
	}
}

func runtimeRecordingErrorMessage(err error) string {
	switch {
	case errors.Is(err, ErrRecordingNoStorage):
		return "recording storage is not configured"
	case errors.Is(err, ErrRecordingUnavailable):
		return "recording runtime is unavailable"
	case errors.Is(err, ErrRecordingStorageFull):
		return "recording storage reserve cannot be restored"
	case errors.Is(err, ErrSecretStoreUnavailable):
		return "camera credential store is unavailable"
	default:
		return "recording could not start"
	}
}

func waitRuntime(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		delay = time.Millisecond
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func timePtr(value time.Time) *time.Time {
	copy := value
	return &copy
}
