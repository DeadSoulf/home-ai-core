package nvr

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/state"
)

type CameraStore interface {
	CreateNVRCamera(
		context.Context,
		string, string, string, string, string, string, string,
		bool,
		time.Time,
	) (state.NVRCameraRecord, error)
	UpdateNVRCamera(
		context.Context,
		string, string, string, string, string, string, string,
		bool, bool,
		time.Time,
	) (state.NVRCameraRecord, error)
	DeleteNVRCamera(context.Context, string) error
	NVRCamera(context.Context, string) (state.NVRCameraRecord, error)
	ListNVRCameras(context.Context) ([]state.NVRCameraRecord, error)
	ListNVRStreamProfiles(context.Context, string) ([]state.NVRStreamProfileRecord, error)
	DeleteNVRStreamProfile(context.Context, string, string) error
	SetNVRStreamProfile(
		context.Context,
		string, string, string, string,
		int, int, float64, int64,
		time.Time,
	) (state.NVRStreamProfileRecord, error)
	SetNVRONVIFSource(
		context.Context,
		string, string, string, string,
		time.Time,
	) (state.NVRONVIFSourceRecord, error)
	NVRONVIFSource(context.Context, string) (state.NVRONVIFSourceRecord, error)
	ActiveNVRStorageTarget(context.Context) (state.NVRStorageTargetRecord, error)
	CreateNVRRecordingSegment(
		context.Context,
		string, string,
		time.Time, time.Time,
		string, string,
		int, int, int64,
		bool, string,
		time.Time,
	) (state.NVRRecordingSegmentRecord, error)
	OldestNVRRetentionSegments(context.Context, string, int) ([]state.NVRRecordingSegmentRecord, error)
	DeleteNVRRecordingSegment(context.Context, string) error
}

type Service struct {
	store       CameraStore
	credentials CameraCredentialStore
	prober      CameraProber
	discoverer  ONVIFDiscoverer
	onvif       ONVIFClient
	now         func() time.Time

	runtimeMu        sync.RWMutex
	supervisorCancel context.CancelFunc
	workers          map[string]cameraWorker
	runtime          map[string]CameraRuntimeStatus
	runtimeEvent     func(RuntimeEvent)
	healthInterval   time.Duration
	retryDelays      []time.Duration

	liveMu          sync.Mutex
	liveSource      LiveSource
	liveSessions    map[string]*liveSession
	liveIdleTimeout time.Duration

	recordingMu       sync.Mutex
	recorder          RecorderSource
	recordingSessions map[string]*recordingSession
	recordingStatus   map[string]RecordingStatus
	spaceChecker      SpaceChecker
	mountChecker      MountChecker
}

type CameraInput struct {
	Name             string `json:"name"`
	Address          string `json:"address"`
	SubstreamAddress string `json:"substream_address,omitempty"`
	Username         string `json:"username,omitempty"`
	Password         string `json:"password,omitempty"`
	Transport        string `json:"transport,omitempty"`
	RecordingMode    string `json:"recording_mode,omitempty"`
	AudioEnabled     bool   `json:"audio_enabled"`
	Enabled          *bool  `json:"enabled,omitempty"`
	ClearCredential  bool   `json:"clear_credentials,omitempty"`
	ClearSubstream   bool   `json:"clear_substream,omitempty"`
}

type CameraConfig struct {
	CameraSummary
	Address          string          `json:"address"`
	SubstreamAddress string          `json:"substream_address,omitempty"`
	Profiles         []StreamProfile `json:"profiles,omitempty"`
}

func NewService(stateDir string, store CameraStore) (*Service, error) {
	if store == nil {
		return nil, errors.New("NVR camera store is required")
	}
	credentials, err := NewFileCredentialStore(stateDir)
	if err != nil {
		return nil, err
	}
	return newService(store, credentials, NewFFProbe()), nil
}

func NewServiceWithDependencies(
	store CameraStore,
	credentials CameraCredentialStore,
	prober CameraProber,
) *Service {
	return newService(store, credentials, prober)
}

func NewServiceWithRuntimeDependencies(
	store CameraStore,
	credentials CameraCredentialStore,
	prober CameraProber,
	liveSource LiveSource,
) *Service {
	service := newService(store, credentials, prober)
	service.liveSource = liveSource
	return service
}

func NewServiceWithONVIFDependencies(
	store CameraStore,
	credentials CameraCredentialStore,
	prober CameraProber,
	discoverer ONVIFDiscoverer,
	onvif ONVIFClient,
) *Service {
	service := newService(store, credentials, prober)
	service.discoverer = discoverer
	service.onvif = onvif
	return service
}

func newService(
	store CameraStore,
	credentials CameraCredentialStore,
	prober CameraProber,
) *Service {
	return &Service{
		store:             store,
		credentials:       credentials,
		prober:            prober,
		discoverer:        NewWSDiscovery(),
		onvif:             NewSOAPONVIFClient(),
		now:               time.Now,
		workers:           map[string]cameraWorker{},
		runtime:           map[string]CameraRuntimeStatus{},
		healthInterval:    30 * time.Second,
		retryDelays:       []time.Duration{2 * time.Second, 5 * time.Second, 15 * time.Second, 30 * time.Second},
		liveSource:        NewFFmpegMJPEGSource(),
		liveSessions:      map[string]*liveSession{},
		liveIdleTimeout:   5 * time.Second,
		recorder:          NewFFmpegSegmentRecorder(),
		recordingSessions: map[string]*recordingSession{},
		recordingStatus:   map[string]RecordingStatus{},
		spaceChecker:      osSpaceChecker{},
		mountChecker:      osMountChecker{},
	}
}

func (s *Service) SecretStoreReady() bool {
	return s != nil && s.credentials != nil
}

func (s *Service) MediaProbeReady() bool {
	return s != nil && s.prober != nil && s.prober.Available()
}

func (s *Service) ONVIFReady() bool {
	return s != nil && s.discoverer != nil && s.onvif != nil
}

func (s *Service) DiscoverONVIF(ctx context.Context) ([]ONVIFDevice, error) {
	if s == nil || s.discoverer == nil {
		return nil, ErrONVIFDiscoveryUnavailable
	}
	return s.discoverer.Discover(ctx)
}

func (s *Service) DiscoverCameras(
	ctx context.Context,
	input CameraDiscoveryInput,
) ([]CameraDiscoveryDevice, error) {
	if s == nil {
		return nil, ErrONVIFDiscoveryUnavailable
	}
	return NewCameraDiscoveryEngine(s.discoverer).DiscoverWithInput(ctx, input)
}

func (s *Service) ONVIFProfiles(
	ctx context.Context,
	input ONVIFProfileRequest,
) ([]ONVIFProfile, error) {
	if s == nil || s.onvif == nil {
		return nil, ErrONVIFConnection
	}
	credential := CameraCredential{
		Username: strings.TrimSpace(input.Username),
		Password: input.Password,
	}
	if credential.Username == "" && credential.Password != "" {
		return nil, errors.New("camera username is required when a password is supplied")
	}
	return s.onvif.Profiles(ctx, strings.TrimSpace(input.Address), credential)
}

func (s *Service) ImportONVIFCamera(
	ctx context.Context,
	createdBy string,
	input ONVIFImportInput,
) (CameraConfig, ProbeResult, error) {
	if s == nil || s.onvif == nil || s.prober == nil {
		return CameraConfig{}, ProbeResult{}, ErrONVIFConnection
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Address = strings.TrimSpace(input.Address)
	input.Username = strings.TrimSpace(input.Username)
	input.MainProfileToken = strings.TrimSpace(input.MainProfileToken)
	input.SubProfileToken = strings.TrimSpace(input.SubProfileToken)
	if input.Name == "" {
		return CameraConfig{}, ProbeResult{}, errors.New("camera name is required")
	}
	if input.MainProfileToken == "" {
		return CameraConfig{}, ProbeResult{}, errors.New("ONVIF main profile is required")
	}
	if input.SubProfileToken != "" && input.SubProfileToken == input.MainProfileToken {
		return CameraConfig{}, ProbeResult{}, errors.New("ONVIF main and sub profiles must be different")
	}
	transport, err := normalizeTransport(input.Transport)
	if err != nil {
		return CameraConfig{}, ProbeResult{}, err
	}
	recordingMode := strings.ToLower(strings.TrimSpace(input.RecordingMode))
	if recordingMode == "" {
		recordingMode = "off"
	}
	switch recordingMode {
	case "off", "continuous", "motion":
	default:
		return CameraConfig{}, ProbeResult{}, errors.New("camera recording mode must be off, continuous or motion")
	}
	credential := CameraCredential{Username: input.Username, Password: input.Password}
	if credential.Username == "" && credential.Password != "" {
		return CameraConfig{}, ProbeResult{}, errors.New("camera username is required when a password is supplied")
	}
	profiles, err := s.onvif.Profiles(ctx, input.Address, credential)
	if err != nil {
		return CameraConfig{}, ProbeResult{}, err
	}
	var mainProfile, subProfile *ONVIFProfile
	for i := range profiles {
		switch profiles[i].Token {
		case input.MainProfileToken:
			copy := profiles[i]
			mainProfile = &copy
		case input.SubProfileToken:
			copy := profiles[i]
			subProfile = &copy
		}
	}
	if mainProfile == nil {
		return CameraConfig{}, ProbeResult{}, errors.New("selected ONVIF main profile is no longer available")
	}
	if input.SubProfileToken != "" && subProfile == nil {
		return CameraConfig{}, ProbeResult{}, errors.New("selected ONVIF sub profile is no longer available")
	}

	probe, err := s.probeStreams(
		ctx,
		mainProfile.StreamURI,
		func() string {
			if subProfile == nil {
				return ""
			}
			return subProfile.StreamURI
		}(),
		transport,
		credential,
	)
	if err != nil {
		return CameraConfig{}, ProbeResult{}, err
	}

	var ref SecretRef
	if credential.Username != "" || credential.Password != "" {
		if s.credentials == nil {
			return CameraConfig{}, ProbeResult{}, ErrSecretStoreUnavailable
		}
		ref, err = s.credentials.PutCameraCredential(ctx, "pending-"+strings.TrimSpace(createdBy), credential)
		if err != nil {
			return CameraConfig{}, ProbeResult{}, err
		}
	}
	cleanupSecret := func() {
		if ref != "" && s.credentials != nil {
			_ = s.credentials.DeleteCameraCredential(context.WithoutCancel(ctx), ref)
		}
	}

	now := s.now().UTC()
	camera, err := s.store.CreateNVRCamera(
		ctx,
		input.Name,
		"onvif",
		mainProfile.StreamURI,
		string(ref),
		transport,
		recordingMode,
		createdBy,
		input.AudioEnabled,
		now,
	)
	if err != nil {
		cleanupSecret()
		return CameraConfig{}, ProbeResult{}, err
	}
	cleanupCamera := func() {
		_ = s.store.DeleteNVRCamera(context.WithoutCancel(ctx), camera.ID)
		cleanupSecret()
	}

	if _, err := s.store.SetNVRStreamProfile(
		ctx, camera.ID, "main", mainProfile.StreamURI,
		probe.Codec, probe.Width, probe.Height, probe.FPS, probe.BitrateBPS, now,
	); err != nil {
		cleanupCamera()
		return CameraConfig{}, ProbeResult{}, err
	}
	if subProfile != nil && probe.Substream != nil {
		if _, err := s.store.SetNVRStreamProfile(
			ctx, camera.ID, "sub", subProfile.StreamURI,
			probe.Substream.Codec, probe.Substream.Width, probe.Substream.Height,
			probe.Substream.FPS, probe.Substream.BitrateBPS, now,
		); err != nil {
			cleanupCamera()
			return CameraConfig{}, ProbeResult{}, err
		}
	}
	if _, err := s.store.SetNVRONVIFSource(
		ctx,
		camera.ID,
		input.Address,
		input.MainProfileToken,
		input.SubProfileToken,
		now,
	); err != nil {
		cleanupCamera()
		return CameraConfig{}, ProbeResult{}, err
	}
	if err := s.RefreshCamera(ctx, camera.ID); err != nil {
		return CameraConfig{}, ProbeResult{}, err
	}
	config, err := s.CameraConfig(ctx, camera.ID)
	if err != nil {
		return CameraConfig{}, ProbeResult{}, err
	}
	return config, probe, nil
}

func (s *Service) TestCamera(ctx context.Context, input CameraInput) (ProbeResult, error) {
	if s == nil || s.prober == nil {
		return ProbeResult{}, ErrMediaRuntimeUnavailable
	}
	address, err := normalizeRTSPAddress(input.Address)
	if err != nil {
		return ProbeResult{}, err
	}
	substreamAddress := strings.TrimSpace(input.SubstreamAddress)
	if substreamAddress != "" {
		substreamAddress, err = normalizeRTSPAddress(substreamAddress)
		if err != nil {
			return ProbeResult{}, fmt.Errorf("invalid camera substream: %w", err)
		}
	}
	transport, err := normalizeTransport(input.Transport)
	if err != nil {
		return ProbeResult{}, err
	}
	credential, err := credentialFromInput(input)
	if err != nil {
		return ProbeResult{}, err
	}
	return s.probeStreams(ctx, address, substreamAddress, transport, credential)
}

func (s *Service) TestExistingCamera(ctx context.Context, cameraID string) (ProbeResult, error) {
	camera, err := s.store.NVRCamera(ctx, strings.TrimSpace(cameraID))
	if err != nil {
		return ProbeResult{}, err
	}
	credential, err := s.resolveCredential(ctx, camera.CredentialRef)
	if err != nil {
		return ProbeResult{}, err
	}
	substreamAddress, err := s.substreamAddress(ctx, camera.ID)
	if err != nil {
		return ProbeResult{}, err
	}
	return s.probeStreams(ctx, camera.Address, substreamAddress, camera.Transport, credential)
}

func (s *Service) CreateCamera(
	ctx context.Context,
	createdBy string,
	input CameraInput,
) (CameraConfig, ProbeResult, error) {
	input, err := normalizeCameraInput(input, true)
	if err != nil {
		return CameraConfig{}, ProbeResult{}, err
	}
	probe, err := s.TestCamera(ctx, input)
	if err != nil {
		return CameraConfig{}, ProbeResult{}, err
	}

	temporaryID := "pending-" + strings.TrimSpace(createdBy)
	credential, err := credentialFromInput(input)
	if err != nil {
		return CameraConfig{}, ProbeResult{}, err
	}
	var ref SecretRef
	if credential.Username != "" || credential.Password != "" {
		if s.credentials == nil {
			return CameraConfig{}, ProbeResult{}, ErrSecretStoreUnavailable
		}
		ref, err = s.credentials.PutCameraCredential(ctx, temporaryID, credential)
		if err != nil {
			return CameraConfig{}, ProbeResult{}, err
		}
	}

	now := s.now().UTC()
	camera, err := s.store.CreateNVRCamera(
		ctx,
		input.Name,
		"rtsp",
		input.Address,
		string(ref),
		input.Transport,
		input.RecordingMode,
		createdBy,
		input.AudioEnabled,
		now,
	)
	if err != nil {
		if ref != "" {
			_ = s.credentials.DeleteCameraCredential(context.WithoutCancel(ctx), ref)
		}
		return CameraConfig{}, ProbeResult{}, err
	}

	// The encrypted payload is intentionally opaque and keyed by the secret
	// reference, so the temporary camera identifier never becomes public.
	if _, err := s.store.SetNVRStreamProfile(
		ctx,
		camera.ID,
		"main",
		camera.Address,
		probe.Codec,
		probe.Width,
		probe.Height,
		probe.FPS,
		probe.BitrateBPS,
		now,
	); err != nil {
		_ = s.store.DeleteNVRCamera(context.WithoutCancel(ctx), camera.ID)
		if ref != "" {
			_ = s.credentials.DeleteCameraCredential(context.WithoutCancel(ctx), ref)
		}
		return CameraConfig{}, ProbeResult{}, err
	}
	if input.SubstreamAddress != "" && probe.Substream != nil {
		if _, err := s.store.SetNVRStreamProfile(
			ctx,
			camera.ID,
			"sub",
			input.SubstreamAddress,
			probe.Substream.Codec,
			probe.Substream.Width,
			probe.Substream.Height,
			probe.Substream.FPS,
			probe.Substream.BitrateBPS,
			now,
		); err != nil {
			_ = s.store.DeleteNVRCamera(context.WithoutCancel(ctx), camera.ID)
			if ref != "" {
				_ = s.credentials.DeleteCameraCredential(context.WithoutCancel(ctx), ref)
			}
			return CameraConfig{}, ProbeResult{}, err
		}
	}
	if err := s.RefreshCamera(ctx, camera.ID); err != nil {
		return CameraConfig{}, ProbeResult{}, err
	}
	config, err := s.CameraConfig(ctx, camera.ID)
	if err != nil {
		return CameraConfig{}, ProbeResult{}, err
	}
	return config, probe, nil
}

func (s *Service) UpdateCamera(
	ctx context.Context,
	cameraID string,
	input CameraInput,
) (CameraConfig, ProbeResult, error) {
	cameraID = strings.TrimSpace(cameraID)
	current, err := s.store.NVRCamera(ctx, cameraID)
	if err != nil {
		return CameraConfig{}, ProbeResult{}, err
	}
	input, err = normalizeCameraInput(input, false)
	if err != nil {
		return CameraConfig{}, ProbeResult{}, err
	}
	enabled := current.Enabled
	if input.Enabled != nil {
		enabled = *input.Enabled
	}

	var credential CameraCredential
	var newRef SecretRef
	oldRef, err := ParseSecretRef(current.CredentialRef)
	if err != nil {
		return CameraConfig{}, ProbeResult{}, err
	}
	switch {
	case input.ClearCredential:
		credential = CameraCredential{}
	case strings.TrimSpace(input.Username) != "" || input.Password != "":
		credential, err = credentialFromInput(input)
		if err != nil {
			return CameraConfig{}, ProbeResult{}, err
		}
	default:
		credential, err = s.resolveCredential(ctx, current.CredentialRef)
		if err != nil {
			return CameraConfig{}, ProbeResult{}, err
		}
	}

	currentSubstream, err := s.substreamAddress(ctx, cameraID)
	if err != nil {
		return CameraConfig{}, ProbeResult{}, err
	}
	targetSubstream := currentSubstream
	switch {
	case input.ClearSubstream:
		targetSubstream = ""
	case input.SubstreamAddress != "":
		targetSubstream = input.SubstreamAddress
	}

	probe, err := s.probeStreams(
		ctx,
		input.Address,
		targetSubstream,
		input.Transport,
		credential,
	)
	if err != nil {
		return CameraConfig{}, ProbeResult{}, err
	}

	targetRef := oldRef
	credentialChanged := input.ClearCredential || strings.TrimSpace(input.Username) != "" || input.Password != ""
	if credentialChanged {
		targetRef = ""
		if credential.Username != "" || credential.Password != "" {
			if s.credentials == nil {
				return CameraConfig{}, ProbeResult{}, ErrSecretStoreUnavailable
			}
			newRef, err = s.credentials.PutCameraCredential(ctx, cameraID, credential)
			if err != nil {
				return CameraConfig{}, ProbeResult{}, err
			}
			targetRef = newRef
		}
	}

	now := s.now().UTC()
	updated, err := s.store.UpdateNVRCamera(
		ctx,
		cameraID,
		input.Name,
		"rtsp",
		input.Address,
		string(targetRef),
		input.Transport,
		input.RecordingMode,
		enabled,
		input.AudioEnabled,
		now,
	)
	if err != nil {
		if newRef != "" {
			_ = s.credentials.DeleteCameraCredential(context.WithoutCancel(ctx), newRef)
		}
		return CameraConfig{}, ProbeResult{}, err
	}
	if _, err := s.store.SetNVRStreamProfile(
		ctx,
		cameraID,
		"main",
		updated.Address,
		probe.Codec,
		probe.Width,
		probe.Height,
		probe.FPS,
		probe.BitrateBPS,
		now,
	); err != nil {
		return CameraConfig{}, ProbeResult{}, err
	}
	if targetSubstream == "" {
		if err := s.store.DeleteNVRStreamProfile(ctx, cameraID, "sub"); err != nil {
			return CameraConfig{}, ProbeResult{}, err
		}
	} else if probe.Substream != nil {
		if _, err := s.store.SetNVRStreamProfile(
			ctx,
			cameraID,
			"sub",
			targetSubstream,
			probe.Substream.Codec,
			probe.Substream.Width,
			probe.Substream.Height,
			probe.Substream.FPS,
			probe.Substream.BitrateBPS,
			now,
		); err != nil {
			return CameraConfig{}, ProbeResult{}, err
		}
	}
	if credentialChanged && oldRef != "" && oldRef != targetRef {
		_ = s.credentials.DeleteCameraCredential(context.WithoutCancel(ctx), oldRef)
	}
	if err := s.RefreshCamera(ctx, cameraID); err != nil {
		return CameraConfig{}, ProbeResult{}, err
	}
	config, err := s.CameraConfig(ctx, cameraID)
	if err != nil {
		return CameraConfig{}, ProbeResult{}, err
	}
	return config, probe, nil
}

func (s *Service) DeleteCamera(ctx context.Context, cameraID string) error {
	camera, err := s.store.NVRCamera(ctx, strings.TrimSpace(cameraID))
	if err != nil {
		return err
	}
	ref, err := ParseSecretRef(camera.CredentialRef)
	if err != nil {
		return err
	}
	if err := s.store.DeleteNVRCamera(ctx, camera.ID); err != nil {
		return err
	}
	s.RemoveCameraRuntime(camera.ID)
	if ref != "" && s.credentials != nil {
		_ = s.credentials.DeleteCameraCredential(context.WithoutCancel(ctx), ref)
	}
	return nil
}

func (s *Service) CameraConfig(ctx context.Context, cameraID string) (CameraConfig, error) {
	camera, err := s.store.NVRCamera(ctx, strings.TrimSpace(cameraID))
	if err != nil {
		return CameraConfig{}, err
	}
	profiles, err := s.store.ListNVRStreamProfiles(ctx, camera.ID)
	if err != nil {
		return CameraConfig{}, err
	}
	out := CameraConfig{
		CameraSummary: summaryFromRecord(camera),
		Address:       camera.Address,
		Profiles:      make([]StreamProfile, 0, len(profiles)),
	}
	out.Runtime = s.CameraRuntime(camera.ID)
	out.Recording = s.CameraRecording(camera.ID)
	for _, profile := range profiles {
		if profile.Role == "sub" {
			out.SubstreamAddress = profile.SourceURI
		}
		out.Profiles = append(out.Profiles, StreamProfile{
			ID:         profile.ID,
			CameraID:   profile.CameraID,
			Role:       profile.Role,
			Codec:      profile.Codec,
			Width:      profile.Width,
			Height:     profile.Height,
			FPS:        profile.FPS,
			BitrateBPS: profile.BitrateBPS,
			CreatedAt:  profile.CreatedAt,
			UpdatedAt:  profile.UpdatedAt,
		})
	}
	return out, nil
}

func summaryFromRecord(camera state.NVRCameraRecord) CameraSummary {
	return CameraSummary{
		ID:             camera.ID,
		Name:           camera.Name,
		Enabled:        camera.Enabled,
		SourceType:     camera.SourceType,
		Transport:      camera.Transport,
		RecordingMode:  camera.RecordingMode,
		AudioEnabled:   camera.AudioEnabled,
		HasCredentials: camera.CredentialRef != "",
		CreatedAt:      camera.CreatedAt,
		UpdatedAt:      camera.UpdatedAt,
	}
}

func normalizeCameraInput(input CameraInput, create bool) (CameraInput, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Address = strings.TrimSpace(input.Address)
	input.SubstreamAddress = strings.TrimSpace(input.SubstreamAddress)
	input.Username = strings.TrimSpace(input.Username)
	input.Transport = strings.ToLower(strings.TrimSpace(input.Transport))
	input.RecordingMode = strings.ToLower(strings.TrimSpace(input.RecordingMode))
	if input.Name == "" {
		return CameraInput{}, errors.New("camera name is required")
	}
	address, err := normalizeRTSPAddress(input.Address)
	if err != nil {
		return CameraInput{}, err
	}
	input.Address = address
	if input.SubstreamAddress != "" {
		input.SubstreamAddress, err = normalizeRTSPAddress(input.SubstreamAddress)
		if err != nil {
			return CameraInput{}, fmt.Errorf("invalid camera substream: %w", err)
		}
	}
	input.Transport, err = normalizeTransport(input.Transport)
	if err != nil {
		return CameraInput{}, err
	}
	if input.RecordingMode == "" {
		input.RecordingMode = "off"
	}
	switch input.RecordingMode {
	case "off", "continuous", "motion":
	default:
		return CameraInput{}, errors.New("camera recording mode must be off, continuous or motion")
	}
	if create && input.ClearCredential {
		return CameraInput{}, errors.New("cannot clear credentials while creating a camera")
	}
	if create && input.ClearSubstream {
		return CameraInput{}, errors.New("cannot clear substream while creating a camera")
	}
	if input.ClearSubstream && input.SubstreamAddress != "" {
		return CameraInput{}, errors.New("camera substream cannot be set and cleared at the same time")
	}
	if _, err := credentialFromInput(input); err != nil {
		return CameraInput{}, err
	}
	return input, nil
}

func (s *Service) probeStreams(
	ctx context.Context,
	mainAddress, substreamAddress, transport string,
	credential CameraCredential,
) (ProbeResult, error) {
	main, err := s.prober.Probe(ctx, ProbeRequest{
		Address:    mainAddress,
		Transport:  transport,
		Credential: credential,
	})
	if err != nil {
		return ProbeResult{}, err
	}
	if strings.TrimSpace(substreamAddress) == "" {
		return main, nil
	}
	sub, err := s.prober.Probe(ctx, ProbeRequest{
		Address:    substreamAddress,
		Transport:  transport,
		Credential: credential,
	})
	if err != nil {
		return ProbeResult{}, fmt.Errorf("camera substream probe failed: %w", err)
	}
	main.Substream = &sub
	return main, nil
}

func (s *Service) substreamAddress(ctx context.Context, cameraID string) (string, error) {
	profiles, err := s.store.ListNVRStreamProfiles(ctx, strings.TrimSpace(cameraID))
	if err != nil {
		return "", err
	}
	for _, profile := range profiles {
		if profile.Role == "sub" {
			return profile.SourceURI, nil
		}
	}
	return "", nil
}

func normalizeTransport(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "tcp", nil
	}
	if value != "tcp" && value != "udp" {
		return "", errors.New("camera transport must be tcp or udp")
	}
	return value, nil
}

func credentialFromInput(input CameraInput) (CameraCredential, error) {
	username := strings.TrimSpace(input.Username)
	if username == "" && input.Password != "" {
		return CameraCredential{}, errors.New("camera username is required when a password is supplied")
	}
	return CameraCredential{Username: username, Password: input.Password}, nil
}

func (s *Service) resolveCredential(ctx context.Context, ref string) (CameraCredential, error) {
	parsed, err := ParseSecretRef(ref)
	if err != nil {
		return CameraCredential{}, err
	}
	if parsed == "" {
		return CameraCredential{}, nil
	}
	if s.credentials == nil {
		return CameraCredential{}, ErrSecretStoreUnavailable
	}
	credential, err := s.credentials.ResolveCameraCredential(ctx, parsed)
	if err != nil {
		return CameraCredential{}, fmt.Errorf("resolve camera credential: %w", err)
	}
	return credential, nil
}
