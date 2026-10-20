package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/homeassistant"
)

const haHealthCommand = `python3 -c "import urllib.request; urllib.request.urlopen('http://127.0.0.1:8123/', timeout=5).close()"`

var haHealthWindow = 5 * time.Minute
var haConfigName = regexp.MustCompile(`^config(?:-[0-9]+)?$`)
var haContainerID = regexp.MustCompile(`^[a-f0-9]{64}$`)

type haState struct {
	Image     string `json:"image"`
	Directory string `json:"directory"`
}

type haContainer struct {
	ID     string `json:"Id"`
	Config struct {
		Image       string            `json:"Image"`
		Labels      map[string]string `json:"Labels"`
		Healthcheck struct {
			Test []string `json:"Test"`
		} `json:"Healthcheck"`
	} `json:"Config"`
	HostConfig struct {
		NetworkMode string `json:"NetworkMode"`
		Privileged  bool   `json:"Privileged"`
	} `json:"HostConfig"`
	State struct {
		Running bool   `json:"Running"`
		Status  string `json:"Status"`
	} `json:"State"`
	Mounts []struct {
		Type, Source, Destination string
		RW                        bool
	} `json:"Mounts"`
	configDir string
}

func haRoot() string { return filepath.Join(moduleDataRoot, homeassistant.ModuleID) }

// Never follow a service-user-controlled symlink into a root-owned host path.
func haRealDirectory(path string, private bool) error {
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) {
		return errors.New("Home Assistant data path is not absolute")
	}
	for p := clean; ; p = filepath.Dir(p) {
		info, err := os.Lstat(p)
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("unsafe Home Assistant directory: %s", p)
		}
		if p == clean && private {
			stat, ok := info.Sys().(*syscall.Stat_t)
			if !ok || stat.Uid != uint32(os.Geteuid()) || info.Mode().Perm()&0o022 != 0 {
				return errors.New("Home Assistant data must be helper-owned and not group/world writable")
			}
		}
		if p == filepath.Dir(p) {
			break
		}
	}
	return nil
}

func haPrepareRoot() error {
	if err := haRealDirectory(moduleDataRoot, true); err != nil {
		return err
	}
	if err := os.Mkdir(haRoot(), 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	return haRealDirectory(haRoot(), true)
}

func haValidateConfig(dir string) error {
	if dir != filepath.Clean(dir) || filepath.Dir(dir) != filepath.Clean(haRoot()) || !haConfigName.MatchString(filepath.Base(dir)) {
		return errors.New("Home Assistant /config is outside its fixed managed directory")
	}
	if err := haRealDirectory(haRoot(), true); err != nil {
		return err
	}
	return haRealDirectory(dir, true)
}

func haReadState() (haState, error) {
	var state haState
	path := filepath.Join(haRoot(), "active.json")
	info, err := os.Lstat(path)
	if err != nil {
		return state, err
	}
	if !info.Mode().IsRegular() || info.Size() > 4096 {
		return state, errors.New("invalid Home Assistant configuration record")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return state, err
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, err
	}
	if err := homeassistant.ValidateImage(state.Image); err != nil {
		return state, err
	}
	return state, haValidateConfig(state.Directory)
}

func haWriteState(filename string, state haState) error {
	if err := homeassistant.ValidateImage(state.Image); err != nil {
		return err
	}
	if err := haValidateConfig(state.Directory); err != nil {
		return err
	}
	if filepath.Base(filename) != filename {
		return errors.New("invalid Home Assistant metadata filename")
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(haRoot(), ".metadata-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(file.Name(), filepath.Join(haRoot(), filename)); err != nil {
		return err
	}
	dir, err := os.Open(haRoot())
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func haInspect(ctx context.Context, binary, name string) (haContainer, error) {
	var info haContainer
	data, err := exec.CommandContext(ctx, binary, "inspect", "--format", "{{json .}}", name).CombinedOutput()
	if err != nil {
		return info, fmt.Errorf("inspect Home Assistant: %s", commandError(data, err))
	}
	if err := json.Unmarshal(data, &info); err != nil {
		return info, err
	}
	if !haContainerID.MatchString(info.ID) {
		return info, errors.New("invalid Home Assistant container identity")
	}
	if info.Config.Labels["home-ai.managed"] != "true" || info.Config.Labels["home-ai.module.id"] != homeassistant.ModuleID {
		return info, errors.New("refusing unmanaged Home Assistant container")
	}
	if err := homeassistant.ValidateImage(info.Config.Image); err != nil {
		return info, err
	}
	if info.HostConfig.NetworkMode != "host" || !info.HostConfig.Privileged {
		return info, errors.New("Home Assistant runtime profile does not match")
	}
	if len(info.Config.Healthcheck.Test) != 2 || info.Config.Healthcheck.Test[0] != "CMD-SHELL" || info.Config.Healthcheck.Test[1] != haHealthCommand {
		return info, errors.New("Home Assistant health check does not match the fixed profile")
	}
	seen := map[string]bool{}
	for _, mount := range info.Mounts {
		if seen[mount.Destination] {
			return info, errors.New("duplicate Home Assistant mount")
		}
		seen[mount.Destination] = true
		if mount.Type != "bind" {
			return info, errors.New("unexpected Home Assistant mount type")
		}
		switch mount.Destination {
		case "/config":
			if !mount.RW {
				return info, errors.New("Home Assistant config mount is read-only")
			}
			if err := haValidateConfig(mount.Source); err != nil {
				return info, err
			}
			info.configDir = mount.Source
		case "/etc/localtime", "/run/dbus":
			if mount.Source != mount.Destination || mount.RW {
				return info, errors.New("unexpected Home Assistant system mount")
			}
		default:
			return info, errors.New("unapproved Home Assistant host mount")
		}
	}
	if info.configDir == "" {
		return info, errors.New("Home Assistant /config mount is missing")
	}
	return info, nil
}

func haCreateArgs(name, image, configDir string, dbus, localtime bool) []string {
	args := []string{"create", "--name", name, "--label", "home-ai.managed=true", "--label", "home-ai.module.id=" + homeassistant.ModuleID,
		"--network", "host", "--privileged", "--restart", "unless-stopped", "--stop-timeout", "60", "--pids-limit", "1024",
		"--mount", "type=bind,src=" + configDir + ",dst=/config", "--health-cmd", haHealthCommand,
		"--health-interval", "5s", "--health-timeout", "8s", "--health-start-period", "60s", "--health-retries", "30"}
	if localtime {
		args = append(args, "--mount", "type=bind,src=/etc/localtime,dst=/etc/localtime,readonly")
	}
	if dbus {
		args = append(args, "--mount", "type=bind,src=/run/dbus,dst=/run/dbus,readonly")
	}
	return append(args, image)
}

func haCreate(ctx context.Context, binary, name, image, configDir string) error {
	if err := homeassistant.ValidateImage(image); err != nil {
		return err
	}
	if err := haValidateConfig(configDir); err != nil {
		return err
	}
	dbus, err := os.Stat("/run/dbus/system_bus_socket")
	hasDBus := err == nil && dbus.Mode()&os.ModeSocket != 0
	_, err = os.Stat("/etc/localtime")
	return dockerAICommand(ctx, binary, haCreateArgs(name, image, configDir, hasDBus, err == nil)...)
}

// The root helper allows AF_UNIX only. Read procfs for preflight instead of
// relaxing that sandbox or opening an arbitrary TCP listener from the helper.
func haPortListening(data []byte) (bool, error) {
	if len(strings.Fields(string(data))) < 4 {
		return false, errors.New("missing kernel TCP table header")
	}
	for n, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if n == 0 {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			return false, errors.New("invalid kernel TCP table")
		}
		parts := strings.Split(fields[1], ":")
		if len(parts) != 2 {
			return false, errors.New("invalid kernel TCP address")
		}
		port, err := strconv.ParseUint(parts[1], 16, 16)
		if err != nil {
			return false, err
		}
		if port == 8123 && fields[3] == "0A" {
			return true, nil
		}
	}
	return false, nil
}

var haPortAvailable = haCheckPort

func haCheckPort() error {
	for _, path := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) && strings.HasSuffix(path, "tcp6") {
			continue
		}
		if err != nil {
			return fmt.Errorf("check Home Assistant port: %w", err)
		}
		busy, err := haPortListening(data)
		if err != nil {
			return err
		}
		if busy {
			return errors.New("host TCP port 8123 is already in use; no existing service was stopped")
		}
	}
	return nil
}

func haPreflight(ctx context.Context, binary string) error {
	if runtime.GOOS != "linux" || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		return errors.New("Home Assistant requires a native Linux amd64 or arm64 host")
	}
	data, err := exec.CommandContext(ctx, binary, "version", "--format", "{{json .Server}}").CombinedOutput()
	if err != nil {
		return fmt.Errorf("inspect Docker server: %s", commandError(data, err))
	}
	var server struct{ Version, Os, Arch string }
	if err := json.Unmarshal(data, &server); err != nil {
		return err
	}
	major, err := strconv.Atoi(strings.Split(server.Version, ".")[0])
	if err != nil || major < 23 || server.Os != "linux" || server.Arch != runtime.GOARCH {
		return errors.New("Home Assistant requires Docker Engine 23+ on the native Linux host architecture")
	}
	info, err := exec.CommandContext(ctx, binary, "info", "--format", "{{.OperatingSystem}}").CombinedOutput()
	if err != nil {
		return fmt.Errorf("inspect Docker host: %s", commandError(info, err))
	}
	if strings.Contains(strings.ToLower(string(info)), "docker desktop") {
		return errors.New("Home Assistant requires native Docker Engine, not Docker Desktop")
	}
	return nil
}

func haPull(ctx context.Context, binary, image string, report moduleProgressFunc) error {
	if err := homeassistant.ValidateImage(image); err != nil {
		return err
	}
	// Anonymous pull with a temporary config; never use the private catalog PAT.
	dir, err := os.MkdirTemp("", "home-ai-ha-pull-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	env := append(os.Environ(), "DOCKER_CONFIG="+dir, "HOME="+dir)
	if err := pullDockerImageWithProgress(ctx, binary, env, image, report); err != nil {
		return err
	}
	data, err := exec.CommandContext(ctx, binary, "image", "inspect", "--format", "{{json .}}", image).CombinedOutput()
	if err != nil {
		return fmt.Errorf("inspect pulled Home Assistant image: %s", commandError(data, err))
	}
	var info struct {
		Os, Architecture string
		RepoDigests      []string
	}
	if err := json.Unmarshal(data, &info); err != nil {
		return err
	}
	if info.Os != "linux" || info.Architecture != runtime.GOARCH {
		return errors.New("Home Assistant image platform does not match host")
	}
	for _, digest := range info.RepoDigests {
		if digest == image {
			return nil
		}
	}
	return errors.New("pulled Home Assistant image does not contain the requested registry digest")
}

func haWaitHealthy(ctx context.Context, binary, name string) error {
	ctx, cancel := context.WithTimeout(ctx, haHealthWindow)
	defer cancel()
	for {
		out, err := exec.CommandContext(ctx, binary, "inspect", "--format", "{{.State.Status}}|{{if .State.Health}}{{.State.Health.Status}}{{end}}", name).CombinedOutput()
		if err != nil {
			return fmt.Errorf("inspect Home Assistant health: %s", commandError(out, err))
		}
		switch strings.TrimSpace(string(out)) {
		case "running|healthy":
			return nil
		case "exited|", "dead|", "running|unhealthy":
			return errors.New("Home Assistant failed its startup health check")
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("Home Assistant did not become ready: %w", ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
}

func haCleanupFailedStart(binary, name string, cause error) error {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err := dockerAICommand(ctx, binary, "stop", "--time", "60", name); err != nil {
		return errors.Join(cause, err)
	}
	return errors.Join(cause, dockerAICommand(ctx, binary, "rm", name))
}

func installHomeAssistant(ctx context.Context, image string, report moduleProgressFunc) (moduleContainerStatus, string, error) {
	if err := homeassistant.ValidateImage(image); err != nil {
		return moduleContainerStatus{}, "", err
	}
	binary, err := exec.LookPath("docker")
	if err != nil {
		return moduleContainerStatus{}, "", err
	}
	if err := haPreflight(ctx, binary); err != nil {
		return moduleContainerStatus{}, "", err
	}
	name := moduleContainerName(homeassistant.ModuleID)
	exists, managed, err := inspectManagedContainer(ctx, binary, name, homeassistant.ModuleID)
	if err != nil {
		return moduleContainerStatus{}, "", err
	}
	if exists {
		if !managed {
			return moduleContainerStatus{}, "", errors.New("Home Assistant name belongs to an unmanaged container")
		}
		info, err := haInspect(ctx, binary, name)
		if err != nil {
			return moduleContainerStatus{}, "", err
		}
		state, err := haReadState()
		if err != nil {
			return moduleContainerStatus{}, "", err
		}
		if info.Config.Image != image || state.Image != image || state.Directory != info.configDir {
			return moduleContainerStatus{}, "", errors.New("Home Assistant install cannot replace an existing image or configuration; use upgrade")
		}
		if !info.State.Running {
			if err := haPortAvailable(); err != nil {
				return moduleContainerStatus{}, "", err
			}
			if err := dockerAICommand(ctx, binary, "start", name); err != nil {
				return moduleContainerStatus{}, "", err
			}
		}
		if err := haWaitHealthy(ctx, binary, name); err != nil {
			return moduleContainerStatus{}, "", err
		}
		status, err := inspectModuleContainer(ctx, binary, name)
		return status, "existing Home Assistant resumed and health checked", err
	}
	candidate, previous := moduleUpgradeStageNames(homeassistant.ModuleID)
	if err := ensureModuleUpgradeNamesFree(ctx, binary, candidate, previous); err != nil {
		return moduleContainerStatus{}, "", err
	}
	if err := haPortAvailable(); err != nil {
		return moduleContainerStatus{}, "", err
	}
	if err := haPrepareRoot(); err != nil {
		return moduleContainerStatus{}, "", err
	}
	state, err := haReadState()
	if errors.Is(err, os.ErrNotExist) {
		state = haState{Image: image, Directory: filepath.Join(haRoot(), "config")}
		// A directory without an intent record is not silently adopted.
		if err := os.Mkdir(state.Directory, 0o700); err != nil {
			return moduleContainerStatus{}, "", fmt.Errorf("prepare fresh Home Assistant config: %w", err)
		}
		if err := haWriteState("active.json", state); err != nil {
			return moduleContainerStatus{}, "", err
		}
	} else if err != nil {
		return moduleContainerStatus{}, "", err
	}
	if state.Image != image {
		return moduleContainerStatus{}, "", errors.New("preserved Home Assistant configuration belongs to another digest; reinstall its exact image before upgrading")
	}
	report("download", "")
	if err := haPull(ctx, binary, image, report); err != nil {
		return moduleContainerStatus{}, "", err
	}
	if err := haPortAvailable(); err != nil {
		return moduleContainerStatus{}, "", err
	}
	report("create", "")
	if err := haCreate(ctx, binary, name, image, state.Directory); err != nil {
		return moduleContainerStatus{}, "", err
	}
	report("start", "")
	if err := dockerAICommand(ctx, binary, "start", name); err != nil {
		return moduleContainerStatus{}, "", haCleanupFailedStart(binary, name, err)
	}
	report("verify", "")
	if err := haWaitHealthy(ctx, binary, name); err != nil {
		return moduleContainerStatus{}, "", haCleanupFailedStart(binary, name, err)
	}
	status, err := inspectModuleContainer(ctx, binary, name)
	return status, "Home Assistant installed; HTTP health check passed", err
}

func controlHomeAssistant(ctx context.Context, action string) (moduleContainerStatus, string, error) {
	if action != "start" && action != "stop" && action != "restart" && action != "remove" {
		return moduleContainerStatus{}, "", errors.New("unsupported Home Assistant operation")
	}
	binary, err := exec.LookPath("docker")
	if err != nil {
		return moduleContainerStatus{}, "", err
	}
	name := moduleContainerName(homeassistant.ModuleID)
	exists, _, err := inspectManagedContainer(ctx, binary, name, homeassistant.ModuleID)
	if err != nil {
		return moduleContainerStatus{}, "", err
	}
	if !exists && action == "remove" {
		return moduleContainerStatus{State: "removed"}, "Home Assistant already absent; configuration preserved", nil
	}
	info, err := haInspect(ctx, binary, name)
	if err != nil {
		return moduleContainerStatus{}, "", err
	}
	candidate, previous := moduleUpgradeStageNames(homeassistant.ModuleID)
	if action != "stop" {
		if err := ensureModuleUpgradeNamesFree(ctx, binary, candidate, previous); err != nil {
			return moduleContainerStatus{}, "", err
		}
	}
	if action == "start" || action == "restart" {
		state, err := haReadState()
		if err != nil {
			return moduleContainerStatus{}, "", err
		}
		if state.Image != info.Config.Image || state.Directory != info.configDir {
			return moduleContainerStatus{}, "", errors.New("Home Assistant has an unfinished upgrade; recover it before starting")
		}
		if !info.State.Running {
			if err := haPortAvailable(); err != nil {
				return moduleContainerStatus{}, "", err
			}
		}
	}
	args := []string{action, name}
	if action == "stop" || action == "restart" {
		args = []string{action, "--time", "60", name}
	}
	if action == "remove" {
		if err := dockerAICommand(ctx, binary, "stop", "--time", "60", name); err != nil {
			return moduleContainerStatus{}, "", err
		}
		args = []string{"rm", name}
	}
	if err := dockerAICommand(ctx, binary, args...); err != nil {
		return moduleContainerStatus{}, "", err
	}
	if action == "remove" {
		return moduleContainerStatus{State: "removed"}, "Home Assistant removed; all configuration generations preserved", nil
	}
	if action == "start" || action == "restart" {
		if err := haWaitHealthy(ctx, binary, name); err != nil {
			return moduleContainerStatus{}, "", err
		}
	}
	status, err := inspectModuleContainer(ctx, binary, name)
	return status, "Home Assistant " + action + " completed", err
}
