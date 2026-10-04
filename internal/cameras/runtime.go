package cameras

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

var (
	ErrSDKUnavailable = errors.New("HCNetSDK runtime is unavailable")
	ErrSDKUnsupported = errors.New("HCNetSDK runtime is unsupported on this platform")
	ErrInvalidTarget  = errors.New("invalid camera target")
)

type SDKStatus struct {
	Architecture string `json:"architecture"`
	Supported    bool   `json:"supported"`
	Available    bool   `json:"available"`
	Initialized  bool   `json:"initialized"`
	LibraryPath  string `json:"library_path,omitempty"`
	Error        string `json:"error,omitempty"`
}

type Status struct {
	ModuleID string    `json:"module_id"`
	Version  string    `json:"version"`
	Backend  string    `json:"backend"`
	SDK      SDKStatus `json:"sdk"`
}

type LoginRequest struct {
	Address  string `json:"address"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type DeviceChannel struct {
	Number int    `json:"number"`
	Kind   string `json:"kind"`
}

type DeviceMetadata struct {
	SerialNumber       string          `json:"serial_number,omitempty"`
	DeviceName         string          `json:"device_name,omitempty"`
	DeviceType         uint16          `json:"device_type"`
	DeviceTypeName     string          `json:"device_type_name,omitempty"`
	Firmware           string          `json:"firmware,omitempty"`
	SoftwareVersion    uint32          `json:"software_version,omitempty"`
	SoftwareBuildDate  uint32          `json:"software_build_date,omitempty"`
	AnalogChannelCount int             `json:"analog_channel_count"`
	IPChannelCount     int             `json:"ip_channel_count"`
	StartAnalogChannel int             `json:"start_analog_channel,omitempty"`
	StartIPChannel     int             `json:"start_ip_channel,omitempty"`
	PasswordLevel      int             `json:"password_level,omitempty"`
	LoginMode          int             `json:"login_mode,omitempty"`
	Channels           []DeviceChannel `json:"channels"`
}

type LoginResult struct {
	OK           bool            `json:"ok"`
	Address      string          `json:"address"`
	Port         int             `json:"port"`
	Backend      string          `json:"backend"`
	SDKErrorCode uint32          `json:"sdk_error_code,omitempty"`
	Device       *DeviceMetadata `json:"device,omitempty"`
}

type SDKError struct {
	Code uint32
}

func (e SDKError) Error() string {
	return fmt.Sprintf("HCNetSDK error %d", e.Code)
}

type sdkRuntime interface {
	Status() SDKStatus
	Refresh() SDKStatus
	TestLogin(context.Context, LoginRequest) (LoginResult, error)
	Close() error
}

type Camera struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Address   string          `json:"address"`
	Port      int             `json:"port"`
	Username  string          `json:"username"`
	Password  string          `json:"-"`
	Backend   string          `json:"backend"`
	CreatedAt string          `json:"created_at"`
	Device    *DeviceMetadata `json:"device,omitempty"`
}

type Service struct {
	runtime     sdkRuntime
	runtimeRoot string
	mu          sync.Mutex
	cameras     []Camera
}

func NewService(stateDir string) *Service {
	runtimeRoot := filepath.Join(stateDir, "hikvision")
	_ = os.Setenv("HOME_AI_HCNETSDK_DIR", runtimeRoot)
	s := &Service{runtime: newHCNetSDKRuntime(), runtimeRoot: runtimeRoot}
	_ = s.loadCameras()
	return s
}

func NewServiceWithRuntime(runtime sdkRuntime) *Service {
	return &Service{runtime: runtime}
}

func (s *Service) Status() Status {
	status := SDKStatus{
		Architecture: runtime.GOARCH,
		Supported:    false,
		Error:        ErrSDKUnavailable.Error(),
	}
	if s != nil && s.runtime != nil {
		status = s.runtime.Status()
	}
	return Status{
		ModuleID: ModuleID,
		Version:  ModuleVersion,
		Backend:  "HCNetSDK",
		SDK:      status,
	}
}

func (s *Service) Refresh() Status {
	if s != nil && s.runtime != nil {
		s.runtime.Refresh()
	}
	return s.Status()
}

func (s *Service) TestLogin(ctx context.Context, request LoginRequest) (LoginResult, error) {
	if s == nil || s.runtime == nil {
		return LoginResult{}, ErrSDKUnavailable
	}
	request.Address = strings.TrimSpace(request.Address)
	request.Username = strings.TrimSpace(request.Username)
	if request.Port == 0 {
		request.Port = 8000
	}
	if err := validateLoginRequest(request); err != nil {
		return LoginResult{}, err
	}
	return s.runtime.TestLogin(ctx, request)
}

func (s *Service) Close() error {
	if s == nil || s.runtime == nil {
		return nil
	}
	return s.runtime.Close()
}

func enumerateDeviceChannels(info DeviceMetadata) []DeviceChannel {
	const maxChannels = 512
	channels := make([]DeviceChannel, 0, info.AnalogChannelCount+info.IPChannelCount)
	appendRange := func(start, count int, kind string) {
		if count <= 0 || len(channels) >= maxChannels {
			return
		}
		if start <= 0 {
			start = 1
		}
		if count > maxChannels-len(channels) {
			count = maxChannels - len(channels)
		}
		for offset := 0; offset < count; offset++ {
			channels = append(channels, DeviceChannel{Number: start + offset, Kind: kind})
		}
	}
	appendRange(info.StartAnalogChannel, info.AnalogChannelCount, "analog")
	appendRange(info.StartIPChannel, info.IPChannelCount, "ip")
	return channels
}

func formatHCNetSDKFirmware(version, buildDate uint32) string {
	if version == 0 && buildDate == 0 {
		return ""
	}
	var value string
	if version>>24 == 0 {
		value = fmt.Sprintf("V%d.%d", (version>>16)&0xffff, version&0xffff)
	} else {
		value = fmt.Sprintf("V%d.%d.%d", version>>24, (version>>16)&0xff, version&0xffff)
	}
	if buildDate != 0 {
		value += fmt.Sprintf(" build %04d%02d%02d", (buildDate>>16)&0xffff, (buildDate>>8)&0xff, buildDate&0xff)
	}
	return value
}

func cleanHCNetSDKText(value string) string {
	return strings.TrimSpace(strings.ToValidUTF8(value, "�"))
}

func validateLoginRequest(request LoginRequest) error {
	if request.Address == "" {
		return fmt.Errorf("%w: camera address is required", ErrInvalidTarget)
	}
	ip := net.ParseIP(request.Address)
	if ip == nil {
		return fmt.Errorf("%w: camera address must be a literal IP", ErrInvalidTarget)
	}
	if !ip.IsPrivate() && !ip.IsLinkLocalUnicast() {
		return fmt.Errorf("%w: camera address must be private or link-local", ErrInvalidTarget)
	}
	if request.Port < 1 || request.Port > 65535 {
		return fmt.Errorf("%w: camera port must be between 1 and 65535", ErrInvalidTarget)
	}
	if request.Username == "" {
		return fmt.Errorf("%w: username is required", ErrInvalidTarget)
	}
	return nil
}

func (s *Service) Cameras() []Camera {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Camera, len(s.cameras))
	copy(out, s.cameras)
	return out
}

func (s *Service) AddCamera(ctx context.Context, request LoginRequest, name string) (Camera, error) {
	result, err := s.TestLogin(ctx, request)
	if err != nil {
		return Camera{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = request.Address
	}
	item := Camera{
		ID:        fmt.Sprintf("cam-%d", time.Now().UnixNano()),
		Name:      name,
		Address:   result.Address,
		Port:      result.Port,
		Username:  request.Username,
		Password:  request.Password,
		Backend:   result.Backend,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Device:    result.Device,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.cameras {
		if existing.Address == item.Address && existing.Port == item.Port {
			return Camera{}, fmt.Errorf("%w: camera already added", ErrInvalidTarget)
		}
	}
	s.cameras = append(s.cameras, item)
	if err := s.saveCamerasLocked(); err != nil {
		s.cameras = s.cameras[:len(s.cameras)-1]
		return Camera{}, err
	}
	return item, nil
}

func (s *Service) cameraStorePath() string { return filepath.Join(s.runtimeRoot, "cameras.json") }

func (s *Service) loadCameras() error {
	data, err := os.ReadFile(s.cameraStorePath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &s.cameras)
}

func (s *Service) saveCamerasLocked() error {
	if err := os.MkdirAll(s.runtimeRoot, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.cameras, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.cameraStorePath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, s.cameraStorePath())
}
