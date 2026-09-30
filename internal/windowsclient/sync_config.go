package windowsclient

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	syncConfigVersion  = 1
	maxSyncConfigBytes = 4 << 20
	minSyncInterval    = time.Minute
	maxSyncInterval    = 30 * 24 * time.Hour
)

type SyncProfile struct {
	ID             string     `json:"id"`
	ServerURL      string     `json:"server_url"`
	Username       string     `json:"username"`
	FolderID       string     `json:"folder_id"`
	Source         string     `json:"source"`
	Destination    string     `json:"destination"`
	EverySeconds   int64      `json:"every_seconds"`
	ConflictPolicy string     `json:"conflict_policy"`
	Enabled        bool       `json:"enabled"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	LastAttemptAt  *time.Time `json:"last_attempt_at,omitempty"`
	LastSuccessAt  *time.Time `json:"last_success_at,omitempty"`
	LastError      string     `json:"last_error,omitempty"`
}

type SyncProfileInput struct {
	ServerURL      string
	Username       string
	FolderID       string
	Source         string
	Destination    string
	Every          time.Duration
	ConflictPolicy string
}

type syncConfigState struct {
	Version  int           `json:"version"`
	Profiles []SyncProfile `json:"profiles"`
}

func (profile SyncProfile) Interval() time.Duration {
	return time.Duration(profile.EverySeconds) * time.Second
}

func (profile SyncProfile) NextDue() time.Time {
	if profile.LastAttemptAt == nil {
		return time.Time{}
	}
	return profile.LastAttemptAt.Add(profile.Interval())
}

func LoadSyncProfiles(filename string) ([]SyncProfile, error) {
	filename, err := normalizeSyncConfigFilename(filename)
	if err != nil {
		return nil, err
	}
	state, err := readSyncConfig(filename)
	if err != nil {
		return nil, err
	}
	return append([]SyncProfile(nil), state.Profiles...), nil
}

func AddSyncProfile(filename string, input SyncProfileInput) (SyncProfile, error) {
	server, err := normalizeQueueServer(input.ServerURL)
	if err != nil {
		return SyncProfile{}, err
	}
	username := strings.TrimSpace(input.Username)
	folderID := strings.TrimSpace(input.FolderID)
	if username == "" || folderID == "" {
		return SyncProfile{}, errors.New("username and folder ID are required")
	}
	if strings.TrimSpace(input.Source) == "" {
		return SyncProfile{}, errors.New("source path is required")
	}
	source, err := filepath.Abs(input.Source)
	if err != nil {
		return SyncProfile{}, fmt.Errorf("resolve sync source: %w", err)
	}
	info, err := os.Lstat(source)
	if err != nil {
		return SyncProfile{}, fmt.Errorf("inspect sync source: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
		return SyncProfile{}, errors.New("sync source must be a regular file or real directory")
	}
	destination := strings.TrimSpace(input.Destination)
	if destination == "" {
		destination = filepath.Base(source)
	}
	destination, err = normalizeCopyDestination(destination)
	if err != nil {
		return SyncProfile{}, err
	}
	if input.Every < minSyncInterval || input.Every > maxSyncInterval {
		return SyncProfile{}, fmt.Errorf("sync interval must be between %s and %s", minSyncInterval, maxSyncInterval)
	}
	policy, err := NormalizeSyncConflictPolicy(input.ConflictPolicy)
	if err != nil {
		return SyncProfile{}, err
	}
	id, err := queueID("sync")
	if err != nil {
		return SyncProfile{}, err
	}
	now := time.Now().UTC()
	profile := SyncProfile{
		ID:             id,
		ServerURL:      server,
		Username:       username,
		FolderID:       folderID,
		Source:         source,
		Destination:    destination,
		EverySeconds:   int64(input.Every / time.Second),
		ConflictPolicy: policy,
		Enabled:        true,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	err = updateSyncConfig(filename, func(state *syncConfigState) error {
		for _, existing := range state.Profiles {
			if existing.ServerURL == profile.ServerURL &&
				existing.Username == profile.Username &&
				existing.FolderID == profile.FolderID &&
				existing.Source == profile.Source &&
				existing.Destination == profile.Destination {
				return errors.New("an equivalent sync profile already exists")
			}
		}
		state.Profiles = append(state.Profiles, profile)
		return nil
	})
	if err != nil {
		return SyncProfile{}, err
	}
	return profile, nil
}

func RemoveSyncProfile(filename, profileID string) error {
	profileID = strings.TrimSpace(profileID)
	if profileID == "" {
		return errors.New("sync profile ID is required")
	}
	return updateSyncConfig(filename, func(state *syncConfigState) error {
		for i := range state.Profiles {
			if state.Profiles[i].ID == profileID {
				state.Profiles = append(state.Profiles[:i], state.Profiles[i+1:]...)
				return nil
			}
		}
		return fmt.Errorf("sync profile %s not found", profileID)
	})
}

func SetSyncProfileEnabled(filename, profileID string, enabled bool) error {
	profileID = strings.TrimSpace(profileID)
	if profileID == "" {
		return errors.New("sync profile ID is required")
	}
	return updateSyncConfig(filename, func(state *syncConfigState) error {
		for i := range state.Profiles {
			if state.Profiles[i].ID == profileID {
				state.Profiles[i].Enabled = enabled
				state.Profiles[i].UpdatedAt = time.Now().UTC()
				return nil
			}
		}
		return fmt.Errorf("sync profile %s not found", profileID)
	})
}

func RecordSyncProfileResult(filename, profileID string, attemptedAt time.Time, runErr error) error {
	attemptedAt = attemptedAt.UTC()
	return updateSyncConfig(filename, func(state *syncConfigState) error {
		for i := range state.Profiles {
			profile := &state.Profiles[i]
			if profile.ID != profileID {
				continue
			}
			profile.LastAttemptAt = timePointer(attemptedAt)
			profile.UpdatedAt = attemptedAt
			if runErr == nil {
				profile.LastSuccessAt = timePointer(attemptedAt)
				profile.LastError = ""
			} else {
				profile.LastError = runErr.Error()
				if len(profile.LastError) > 4096 {
					profile.LastError = profile.LastError[:4096]
				}
			}
			return nil
		}
		return fmt.Errorf("sync profile %s not found", profileID)
	})
}

func DueSyncProfiles(profiles []SyncProfile, now time.Time) []SyncProfile {
	now = now.UTC()
	result := make([]SyncProfile, 0, len(profiles))
	for _, profile := range profiles {
		if !profile.Enabled {
			continue
		}
		next := profile.NextDue()
		if next.IsZero() || !now.Before(next) {
			result = append(result, profile)
		}
	}
	return result
}

func NextSyncDue(profiles []SyncProfile) (time.Time, bool) {
	var next time.Time
	found := false
	for _, profile := range profiles {
		if !profile.Enabled {
			continue
		}
		candidate := profile.NextDue()
		if candidate.IsZero() {
			return time.Time{}, true
		}
		if !found || candidate.Before(next) {
			next, found = candidate, true
		}
	}
	return next, found
}

func updateSyncConfig(filename string, mutate func(*syncConfigState) error) error {
	filename, err := normalizeSyncConfigFilename(filename)
	if err != nil {
		return err
	}
	lockName := filename + ".lock"
	if info, err := os.Lstat(lockName); err == nil {
		if !info.Mode().IsRegular() {
			return errors.New("sync profile lock path must be a regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect sync profile lock: %w", err)
	}
	lock, err := lockQueueFile(lockName)
	if err != nil {
		return fmt.Errorf("lock sync profiles: %w", err)
	}
	defer func() {
		_ = unlockQueueFile(lock)
		_ = lock.Close()
	}()
	state, err := readSyncConfig(filename)
	if err != nil {
		return err
	}
	if err := mutate(&state); err != nil {
		return err
	}
	if err := validateSyncConfig(state); err != nil {
		return fmt.Errorf("invalid sync profile update: %w", err)
	}
	return persistSyncConfig(filename, state)
}

func normalizeSyncConfigFilename(filename string) (string, error) {
	if strings.TrimSpace(filename) == "" {
		return "", errors.New("sync profile path is required")
	}
	filename, err := filepath.Abs(filename)
	if err != nil {
		return "", fmt.Errorf("resolve sync profile path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
		return "", fmt.Errorf("create sync profile directory: %w", err)
	}
	dir, err := filepath.EvalSymlinks(filepath.Dir(filename))
	if err != nil {
		return "", fmt.Errorf("resolve sync profile directory: %w", err)
	}
	filename = filepath.Join(dir, filepath.Base(filename))
	if info, err := os.Lstat(filename); err == nil {
		if !info.Mode().IsRegular() {
			return "", errors.New("sync profile path must be a regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect sync profile path: %w", err)
	}
	return filename, nil
}

func readSyncConfig(filename string) (syncConfigState, error) {
	state := syncConfigState{Version: syncConfigVersion, Profiles: []SyncProfile{}}
	file, err := os.Open(filename)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return syncConfigState{}, fmt.Errorf("open sync profiles: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return syncConfigState{}, fmt.Errorf("inspect sync profiles: %w", err)
	}
	if info.Size() > maxSyncConfigBytes {
		return syncConfigState{}, errors.New("sync profile file exceeds 4 MiB")
	}
	decoder := json.NewDecoder(io.LimitReader(file, maxSyncConfigBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return syncConfigState{}, fmt.Errorf("decode sync profiles: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return syncConfigState{}, errors.New("sync profile file must contain exactly one JSON object")
	}
	if err := validateSyncConfig(state); err != nil {
		return syncConfigState{}, fmt.Errorf("invalid sync profiles: %w", err)
	}
	return state, nil
}

func persistSyncConfig(filename string, state syncConfigState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode sync profiles: %w", err)
	}
	data = append(data, '\n')
	if len(data) > maxSyncConfigBytes {
		return errors.New("sync profile file exceeds 4 MiB")
	}
	file, err := os.CreateTemp(filepath.Dir(filename), ".home-ai-sync-*")
	if err != nil {
		return fmt.Errorf("create sync profile checkpoint: %w", err)
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write sync profile checkpoint: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync profile checkpoint: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close sync profile checkpoint: %w", err)
	}
	if err := replaceQueueFile(name, filename); err != nil {
		return fmt.Errorf("replace sync profile checkpoint: %w", err)
	}
	return nil
}

func validateSyncConfig(state syncConfigState) error {
	if state.Version != syncConfigVersion {
		return fmt.Errorf("unsupported sync profile version %d", state.Version)
	}
	if state.Profiles == nil {
		return errors.New("profiles must be an array")
	}
	ids := map[string]bool{}
	for _, profile := range state.Profiles {
		if profile.ID == "" || ids[profile.ID] {
			return errors.New("invalid or duplicate sync profile ID")
		}
		ids[profile.ID] = true
		server, err := normalizeQueueServer(profile.ServerURL)
		if err != nil || server != profile.ServerURL {
			return errors.New("invalid sync profile server URL")
		}
		if strings.TrimSpace(profile.Username) == "" || strings.TrimSpace(profile.Username) != profile.Username ||
			strings.TrimSpace(profile.FolderID) == "" || strings.TrimSpace(profile.FolderID) != profile.FolderID {
			return errors.New("invalid sync profile username or folder ID")
		}
		if !filepath.IsAbs(profile.Source) {
			return errors.New("sync profile source must be absolute")
		}
		if clean, err := normalizeCopyDestination(profile.Destination); err != nil || clean != profile.Destination {
			return errors.New("invalid sync profile destination")
		}
		interval := profile.Interval()
		if interval < minSyncInterval || interval > maxSyncInterval {
			return errors.New("invalid sync profile interval")
		}
		if policy, err := NormalizeSyncConflictPolicy(profile.ConflictPolicy); err != nil || policy != profile.ConflictPolicy {
			return errors.New("invalid sync profile conflict policy")
		}
		if profile.CreatedAt.IsZero() || profile.UpdatedAt.IsZero() || profile.UpdatedAt.Before(profile.CreatedAt) {
			return errors.New("invalid sync profile timestamps")
		}
		if len(profile.LastError) > 4096 {
			return errors.New("sync profile error is too long")
		}
	}
	return nil
}

func timePointer(value time.Time) *time.Time {
	copy := value
	return &copy
}
